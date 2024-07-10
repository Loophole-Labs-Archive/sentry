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

	ctx    context.Context
	cancel context.CancelFunc

	activeConn io.ReadWriteCloser

	logger logging.Logger
	wg     sync.WaitGroup
}

func NewServer(ctx context.Context, logger logging.Logger) *Server {
	c := &Server{
		ctx:                ctx,
		priorityWriteQueue: make(chan *InflightRequest, MaximumQueueSize),
		writeQueue:         make(chan *InflightRequest, MaximumQueueSize),
		inflight:           make(map[uuid.UUID]*InflightRequest),
		logger:             logger,
	}
	return c
}

func (s *Server) HandleConnection(conn io.ReadWriteCloser) {
	s.activeConn = conn
	s.ctx, s.cancel = context.WithCancel(s.externalCtx)

	s.wg.Add(1)
	go s.write()

	s.wg.Add(1)
	go s.read()

	<-s.ctx.Done()
	s.logger.Infof("shutting down connection")
	_ = s.activeConn.Close()
	s.logger.Infof("connection was closed")
	s.wg.Wait()
	s.logger.Infof("read and write loops have shut down")
}

func (s *Server) DoRPC(request *Request, response *Response) (err error) {
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

	timeout := time.NewTimer(Timeout)

	select {
	case <-s.ctx.Done():
		s.logger.Warnf("context was canceled, cancelling request %s of type %d", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		err = context.Canceled
	case <-inflightRequest.complete:
		s.logger.Infof("request %s of type %d was completed", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		err = nil
	case <-timeout.C:
		s.logger.Warnf("request %s of type %d timed out", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		err = context.DeadlineExceeded
	}

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
			s.logger.Error("write queue was closed")
			goto OUT
		}
		s.logger.Infof("writing request %s of type %d (priority %t)", inflightRequest.Request.UUID, inflightRequest.Request.Type, priority)
		_, err := s.activeConn.Write(inflightRequest.RequestBuffer.Bytes())
		if err != nil {
			s.logger.Errorf("unable to write request: %v", err)
			select {
			case s.priorityWriteQueue <- inflightRequest:
				s.logger.Infof("requeueing request %s of type %d for priority writing", inflightRequest.Request.UUID, inflightRequest.Request.Type)
			default:
				s.logger.Warnf("priority write queue is full, dropping request %s of type %d", inflightRequest.Request.UUID, inflightRequest.Request.Type)
			}
			break
		}
	}
OUT:
	s.logger.Infof("shutting down write loop")
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
			s.logger.Errorf("unable to read from connection: %v", err)
			goto OUT
		}
		_uuid, err = DecodeUUID(buf[:n])
		if err != nil {
			s.logger.Errorf("unable to decode UUID: %v", err)
			continue
		}
		s.inflightMutex.RLock()
		inflightRequest, ok = s.inflight[_uuid]
		s.inflightMutex.RUnlock()
		if !ok {
			s.logger.Errorf("unknown request UUID: %s", _uuid)
			continue
		}
		err = inflightRequest.Response.Decode(buf[:n])
		if err != nil {
			inflightRequest.Response.UUID = _uuid
			s.logger.Errorf("unable to decode response for request %s: %v", err, inflightRequest.Response.UUID)
			inflightRequest.Request.Data = nil
			inflightRequest.Response.Error = err
			close(inflightRequest.complete)
			continue
		}
		s.logger.Infof("handling response for request %s of type %d for processing", inflightRequest.Request.UUID, inflightRequest.Request.Type)
		close(inflightRequest.complete)
	}
OUT:
	s.logger.Infof("shutting down read loop")
	s.cancel()
	s.wg.Done()
}
