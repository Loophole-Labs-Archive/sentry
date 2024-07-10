// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/loopholelabs/logging"
	"github.com/loopholelabs/polyglot/v2"
)

const (
	Timeout = 5 * time.Second
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
		logger:             logger,
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
	s.logger.Infof("[Server] shutting down connection\n")
	_ = s.activeConn.Close()
	s.logger.Infof("[Server] connection was closed\n")
	s.wg.Wait()
	s.logger.Infof("[Server] read and write loops have shut down\n")
}

func (s *Server) DoRPC(ctx context.Context, request *Request, response *Response) (err error) {
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
	case s.writeQueue <- inflightRequest:
	case <-ctx.Done():
		err = context.Canceled
		goto OUT
	case <-s.externalCtx.Done():
		s.logger.Warnf("[Server] context was canceled, cancelling request %s of type %d\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		err = context.Canceled
		goto OUT
	}

	select {
	case <-ctx.Done():
		err = context.Canceled
	case <-s.externalCtx.Done():
		s.logger.Warnf("[Server] context was canceled, cancelling request %s of type %d\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		err = context.Canceled
	case <-inflightRequest.complete:
		s.logger.Infof("[Server] request %s of type %d was completed\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
	}

OUT:
	s.inflightMutex.Lock()
	delete(s.inflight, request.UUID)
	s.inflightMutex.Unlock()

	polyglot.PutBuffer(buffer)
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
			s.logger.Error("[Server] write queue was closed\n")
			goto OUT
		}
		s.logger.Infof("[Server] writing request %s of type %d (priority %t)\n", inflightRequest.Request.UUID, inflightRequest.Request.Type, priority)
		_, err := s.activeConn.Write(inflightRequest.RequestBuffer.Bytes())
		if err != nil {
			s.logger.Errorf("[Server] unable to write request: %v\n", err)
			select {
			case s.priorityWriteQueue <- inflightRequest:
				s.logger.Infof("[Server] requeueing request %s of type %d for priority writing\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
			default:
				s.logger.Warnf("[Server] priority write queue is full, requeue of request %s of type %d for priority writing will block\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
				s.priorityWriteQueue <- inflightRequest
			}
			break
		}
	}
OUT:
	s.logger.Infof("[Server] shutting down write loop\n")
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
			s.logger.Errorf("[Server] unable to read from connection: %v\n", err)
			goto OUT
		}
		_uuid, err = DecodeUUID(buf[:n])
		if err != nil {
			s.logger.Errorf("[Server] unable to decode UUID: %v\n", err)
			continue
		}
		s.inflightMutex.RLock()
		inflightRequest, ok = s.inflight[_uuid]
		s.inflightMutex.RUnlock()
		if !ok {
			s.logger.Errorf("[Server] unknown request UUID: %s\n", _uuid)
			continue
		}
		err = inflightRequest.Response.Decode(buf[:n])
		if err != nil {
			inflightRequest.Response.UUID = _uuid
			s.logger.Errorf("[Server] unable to decode response for request %s: %v\n", err, inflightRequest.Response.UUID)
			inflightRequest.Request.Data = nil
			inflightRequest.Response.Error = err
			close(inflightRequest.complete)
			continue
		}
		s.logger.Infof("[Server] handling response for request %s of type %d for processing\n", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		close(inflightRequest.complete)
	}
OUT:
	s.logger.Infof("[Server] shutting down read loop\n")
	s.cancel()
	s.wg.Done()
}
