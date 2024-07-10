// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"io"
	"sync"

	"github.com/loopholelabs/logging"
	"github.com/loopholelabs/polyglot/v2"
)

type Client struct {
	handle HandleRPC

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

func NewClient(handle HandleRPC, logger logging.Logger) *Client {
	c := &Client{
		handle:             handle,
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
	c.logger.Infof("shutting down connection\n")
	_ = c.activeConn.Close()
	c.logger.Infof("connection was closed\n")
	c.wg.Wait()
	c.logger.Infof("read and write loops have shut down\n")
}

func (c *Client) read() {
	buf := make([]byte, MaximumRequestPacketSize)
	var err error
	var n int
	for {
		n, err = io.ReadAtLeast(c.activeConn, buf, MinimumRequestSize)
		if err != nil {
			c.logger.Errorf("unable to read from connection: %v\n", err)
			goto OUT
		}
		processRequest := new(ProcessRequest)
		err = processRequest.Request.Decode(buf[:n])
		if err != nil {
			c.logger.Errorf("unable to decode request: %v\n", err)
			continue
		}
		c.logger.Infof("queueing request %s of type %d for processing\n", processRequest.Request.UUID, processRequest.Request.Type)
		c.processQueue <- processRequest
	}
OUT:
	c.logger.Infof("shutting down read loop\n")
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
			goto OUT
		case processRequest, ok = <-c.priorityWriteQueue:
			priority = true
		case processRequest, ok = <-c.writeQueue:
			priority = false
		}
		if !ok {
			c.logger.Error("write queue was closed\n")
			goto OUT
		}
		c.logger.Infof("writing response for request %s of type %d (priority %t)\n", processRequest.Request.UUID, processRequest.Request.Type, priority)
		_, err := c.activeConn.Write(processRequest.ResponseBuffer.Bytes())
		if err != nil {
			c.logger.Errorf("unable to write response: %v\n", err)
			select {
			case c.priorityWriteQueue <- processRequest:
				c.logger.Infof("requeueing response for request %s of type %d for priority writing\n", processRequest.Request.UUID, processRequest.Request.Type)
			default:
				c.logger.Warnf("priority write queue is full, dropping response for request %s of type %d\n", processRequest.Request.UUID, processRequest.Request.Type)
			}
			goto OUT
		}
		polyglot.PutBuffer(processRequest.ResponseBuffer)
	}
OUT:
	c.logger.Infof("shutting down write loop\n")
	c.cancel()
	c.wg.Done()
}

func (c *Client) worker(id int) {
	var processRequest *ProcessRequest
	var ok bool
	c.logger.Infof("starting worker %d\n", id)
	for {
		processRequest, ok = <-c.processQueue
		if !ok {
			c.logger.Errorf("process queue was closed (worker %d)\n", id)
			goto OUT
		}
		c.logger.Infof("processing request %s of type %d (worker %d)\n", processRequest.Request.UUID, processRequest.Request.Type, id)
		processRequest.Response.UUID = processRequest.Request.UUID
		c.handle(&processRequest.Request, &processRequest.Response)
		processRequest.ResponseBuffer = polyglot.GetBuffer()
		processRequest.Response.Encode(processRequest.ResponseBuffer)
		select {
		case c.writeQueue <- processRequest:
			c.logger.Infof("queueing request %s of type %d for writing\n", processRequest.Request.UUID, processRequest.Request.Type)
		default:
			c.logger.Warnf("write queue is full (worker %d), dropping request %s of type %d\n", id, processRequest.Request.UUID, processRequest.Request.Type)
		}
	}
OUT:
	c.logger.Infof("shutting down worker %d\n", id)
	c.workerWg.Done()
}
