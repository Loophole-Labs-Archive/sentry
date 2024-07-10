// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"errors"
	"github.com/loopholelabs/logging"
	"github.com/loopholelabs/polyglot/v2"
	"io"
	"sync"
)

var (
	UnknownRequestTypeErr = errors.New("unknown request type")
)

type ProcessRequest struct {
	Request  Request
	Response Response
	Buffer   *polyglot.Buffer
}

const (
	MaximumQueueSize         = 1024
	MaximumRequestPacketSize = 1024
	NumWorkers               = 1
)

type Client struct {
	processQueue       chan *ProcessRequest
	priorityWriteQueue chan *ProcessRequest
	writeQueue         chan *ProcessRequest

	activeConn io.ReadWriteCloser
	ctx        context.Context
	cancel     context.CancelFunc

	logger   logging.Logger
	workerWg sync.WaitGroup
	wg       sync.WaitGroup
}

func NewClient(logger logging.Logger) *Client {
	c := &Client{
		processQueue:       make(chan *ProcessRequest, MaximumQueueSize),
		priorityWriteQueue: make(chan *ProcessRequest, MaximumQueueSize),
		writeQueue:         make(chan *ProcessRequest, MaximumQueueSize),
		logger:             logger,
	}
	for i := 0; i < NumWorkers; i++ {
		c.workerWg.Add(1)
		go c.worker(i)
	}
	return c
}

func (c *Client) HandleConnection(conn io.ReadWriteCloser) {
	c.activeConn = conn
	c.ctx, c.cancel = context.WithCancel(context.Background())

	c.wg.Add(1)
	go c.read()

	c.wg.Add(1)
	go c.write()

	<-c.ctx.Done()
	c.logger.Infof("shutting down connection")
	_ = c.activeConn.Close()
	c.logger.Infof("connection was closed")
	c.wg.Wait()
	c.logger.Infof("read and write loops have shut down")
}

func (c *Client) read() {
	buf := make([]byte, MaximumRequestPacketSize)
	var err error
	var n int
	for {
		n, err = io.ReadAtLeast(c.activeConn, buf, MinimumRequestSize)
		if err != nil {
			c.logger.Errorf("unable to read from connection: %v", err)
			break
		}
		processRequest := new(ProcessRequest)
		err = processRequest.Request.Decode(buf[:n])
		if err != nil {
			c.logger.Errorf("unable to decode request: %v", err)
			continue
		}
		c.logger.Infof("queueing request %s of type %d for processing", processRequest.Request.UUID, processRequest.Request.Type)
		c.processQueue <- processRequest
	}
	c.logger.Infof("shutting down read loop")
	c.cancel()
	c.wg.Done()
}

func (c *Client) write() {
	var processRequest *ProcessRequest
	var ok bool
	var priority bool
	for {
		select {
		case <-c.ctx.Done():
			break
		case processRequest, ok = <-c.priorityWriteQueue:
			priority = true
		case processRequest, ok = <-c.writeQueue:
			priority = false
		}
		if !ok {
			c.logger.Error("write queue was closed")
			break
		}
		c.logger.Infof("writing request: %s of type %d (priority %t)", processRequest.Request.UUID, processRequest.Request.Type, priority)
		_, err := c.activeConn.Write(processRequest.Buffer.Bytes())
		if err != nil {
			c.logger.Errorf("unable to write response: %v", err)
			select {
			case c.priorityWriteQueue <- processRequest:
				c.logger.Infof("requeueing request %s of type %d for priority writing", processRequest.Request.UUID, processRequest.Request.Type)
			default:
				c.logger.Warnf("priority write queue is full, dropping request %s of type %d", processRequest.Request.UUID, processRequest.Request.Type)
			}
			break
		}
		polyglot.PutBuffer(processRequest.Buffer)
	}
	c.logger.Infof("shutting down write loop")
	c.cancel()
	c.wg.Done()
}

func (c *Client) worker(id int) {
	var processRequest *ProcessRequest
	var ok bool
	c.logger.Infof("starting worker %d", id)
	for {
		processRequest, ok = <-c.processQueue
		if !ok {
			c.logger.Errorf("process queue was closed (worker %d)", id)
			break
		}
		c.logger.Infof("processing request: %s of type %d (worker %d)", processRequest.Request.UUID, processRequest.Request.Type, id)
		processRequest.Response.UUID = processRequest.Request.UUID
		switch RequestType(processRequest.Request.Type) {
		case RequestTypePing:
			processRequest.Response.Data = processRequest.Request.Data
		default:
			c.logger.Errorf("unknown request type: %d (worker %d)", processRequest.Request.Type, id)
			processRequest.Response.Error = UnknownRequestTypeErr
		}
		processRequest.Buffer = polyglot.GetBuffer()
		processRequest.Response.Encode(processRequest.Buffer)
		select {
		case c.writeQueue <- processRequest:
			c.logger.Infof("queueing request %s of type %d for writing", processRequest.Request.UUID, processRequest.Request.Type)
		default:
			c.logger.Warnf("write queue is full (worker %d), dropping request %s of type %d", id, processRequest.Request.UUID, processRequest.Request.Type)
		}
	}
	c.logger.Infof("shutting down worker %d", id)
	c.workerWg.Done()
}
