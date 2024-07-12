// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"io"
	"sync"

	"github.com/google/uuid"
	logging "github.com/loopholelabs/logging/types"
	"github.com/loopholelabs/polyglot/v2"
)

type Server struct {
	externalCtx        context.Context
	priorityWriteQueue chan *InflightRequest
	writeQueue         chan *InflightRequest
	inflightMutex      sync.RWMutex
	inflight           map[uuid.UUID]*InflightRequest

	activeConn io.ReadWriteCloser
	ctx        context.Context
	cancel     context.CancelFunc

	logger logging.Logger
	wg     sync.WaitGroup
}

func NewServer(ctx context.Context, logger logging.Logger) *Server {
	s := &Server{
		externalCtx:        ctx,
		priorityWriteQueue: make(chan *InflightRequest, MaximumQueueSize),
		writeQueue:         make(chan *InflightRequest, MaximumQueueSize),
		inflight:           make(map[uuid.UUID]*InflightRequest),
		logger:             logger.SubLogger("rpc"),
	}
	return s
}

func (s *Server) HandleConnection(conn io.ReadWriteCloser) {
	s.activeConn = conn
	s.ctx, s.cancel = context.WithCancel(s.externalCtx)

	s.wg.Add(1)
	go s.write()

	s.wg.Add(1)
	go s.read()

	<-s.ctx.Done()
	s.logger.Info().Msg("shutting down connection")
	_ = s.activeConn.Close()
	s.logger.Info().Msg("connection was closed")
	s.wg.Wait()
	s.logger.Info().Msg("write and read loops have shut down")
}

func (s *Server) Do(ctx context.Context, request *Request, response *Response) (err error) {
	buffer := polyglot.GetBuffer()
	request.Encode(buffer)
	inflightRequest := &InflightRequest{
		Request:       request,
		Response:      response,
		RequestBuffer: buffer,
		complete:      make(chan struct{}),
	}
	s.inflightMutex.Lock()
	s.inflight[request.UUID] = inflightRequest
	s.inflightMutex.Unlock()

	select {
	case <-ctx.Done():
		err = context.Canceled
		goto OUT
	case <-s.externalCtx.Done():
		s.logger.Warn().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("context was canceled, cancelling request")
		err = context.DeadlineExceeded
		goto OUT
	case s.writeQueue <- inflightRequest:
		s.logger.Warn().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("request was queued")
	}

	select {
	case <-ctx.Done():
		err = context.Canceled
	case <-s.externalCtx.Done():
		s.logger.Warn().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("context was canceled, cancelling request")
		err = context.DeadlineExceeded
	case <-inflightRequest.complete:
		s.logger.Info().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("request was completed")
	}

OUT:
	s.inflightMutex.Lock()
	delete(s.inflight, request.UUID)
	s.inflightMutex.Unlock()
	return err
}

func (s *Server) write() {
	var inflightRequest *InflightRequest
	var ok bool
	var priority bool
	for {
		select {
		case <-s.ctx.Done():
			goto OUT
		case inflightRequest, ok = <-s.priorityWriteQueue:
			priority = true
		case inflightRequest, ok = <-s.writeQueue:
			priority = false
		}
		if !ok {
			s.logger.Error().Msg("write queue was closed")
			goto OUT
		}
		s.logger.Info().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Bool("priority", priority).Msg("writing request")
		_, err := s.activeConn.Write(inflightRequest.RequestBuffer.Bytes())
		if err != nil {
			s.logger.Error().Err(err).Msg("unable to write request")
			select {
			case s.priorityWriteQueue <- inflightRequest:
				s.logger.Info().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("requeueing request for priority writing")
			default:
				s.logger.Warn().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("priority write queue is full, requeue of request for priority writing will block")
				s.priorityWriteQueue <- inflightRequest
			}
			goto OUT
		}
		polyglot.PutBuffer(inflightRequest.RequestBuffer)
	}
OUT:
	s.logger.Info().Msg("shutting down write loop")
	s.cancel()
	s.wg.Done()
}

func (s *Server) read() {
	buf := make([]byte, MaximumResponsePacketSize)
	var err error
	var n int
	var _uuid uuid.UUID
	var inflightRequest *InflightRequest
	var ok bool
	for {
		n, err = io.ReadAtLeast(s.activeConn, buf, MinimumResponseSize)
		if err != nil {
			s.logger.Error().Err(err).Msg("unable to read from connection")
			goto OUT
		}
		_uuid, err = DecodeUUID(buf[:n])
		if err != nil {
			s.logger.Error().Err(err).Msg("unable to decode UUID")
			continue
		}
		s.inflightMutex.RLock()
		inflightRequest, ok = s.inflight[_uuid]
		s.inflightMutex.RUnlock()
		if !ok {
			s.logger.Error().Str("uuid", _uuid.String()).Msg("unknown request UUID")
			continue
		}
		err = inflightRequest.Response.Decode(buf[:n])
		if err != nil {
			inflightRequest.Response.UUID = _uuid
			s.logger.Error().Str("uuid", inflightRequest.Response.UUID.String()).Err(err).Msg("unable to decode response")
			inflightRequest.Request.Data = nil
			inflightRequest.Response.Error = err
			close(inflightRequest.complete)
			continue
		}
		s.logger.Info().Str("uuid", inflightRequest.Request.UUID.String()).Uint32("type", inflightRequest.Request.Type).Msg("handling response for processing")
		close(inflightRequest.complete)
	}
OUT:
	s.logger.Info().Msg("shutting down read loop")
	s.cancel()
	s.wg.Done()
}
