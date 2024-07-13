// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"io"
	"sync"

	logging "github.com/loopholelabs/logging/types"
	"github.com/loopholelabs/polyglot/v2"
)

type Client struct {
	externalCtx context.Context

	handle HandleFunc

	processQueue       chan *ProcessRequest
	priorityWriteQueue chan *ProcessRequest
	writeQueue         chan *ProcessRequest

	activeConn io.ReadWriteCloser
	ctx        context.Context
	cancel     context.CancelFunc

	lastProcessRequest *ProcessRequest

	logger   logging.SubLogger
	workerWg sync.WaitGroup
	wg       sync.WaitGroup
}

func NewClient(ctx context.Context, handle HandleFunc, logger logging.SubLogger) *Client {
	c := &Client{
		externalCtx:        ctx,
		handle:             handle,
		processQueue:       make(chan *ProcessRequest, MaximumQueueSize),
		priorityWriteQueue: make(chan *ProcessRequest, MaximumQueueSize),
		writeQueue:         make(chan *ProcessRequest, MaximumQueueSize),
		logger:             logger.SubLogger("rpc"),
	}
	for i := 0; i < NumWorkers; i++ {
		c.workerWg.Add(1)
		go c.worker(i)
	}

	return c
}

func (c *Client) HandleConnection(conn io.ReadWriteCloser) {
	c.activeConn = conn
	c.ctx, c.cancel = context.WithCancel(c.externalCtx)

	c.wg.Add(1)
	go c.read()

	c.wg.Add(1)
	go c.write()

	<-c.ctx.Done()
	c.logger.Info().Msg("shutting down connection")
	_ = c.activeConn.Close()
	c.logger.Info().Msg("connection was closed")
	c.wg.Wait()
	c.logger.Info().Msg("read and write loops have shut down")
}

func (c *Client) Close() error {
	c.cancel()
	c.wg.Wait()
	return nil
}

func (c *Client) read() {
	buf := make([]byte, MaximumRequestPacketSize)
	var err error
	var n int
	var processRequest *ProcessRequest
	for {
		if c.lastProcessRequest != nil {
			processRequest = c.lastProcessRequest
			c.lastProcessRequest = nil
			goto QUEUE
		}
		n, err = io.ReadAtLeast(c.activeConn, buf, MinimumRequestSize)
		if err != nil {
			c.logger.Error().Err(err).Msg("unable to read from connection")
			goto OUT
		}
		processRequest = new(ProcessRequest)
		err = processRequest.Request.Decode(buf[:n])
		if err != nil {
			c.logger.Error().Err(err).Msg("unable to decode request")
			continue
		}
		c.logger.Info().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Msg("queueing request for processing")
	QUEUE:
		select {
		case <-c.ctx.Done():
			c.lastProcessRequest = processRequest
			goto OUT
		case c.processQueue <- processRequest:
		}
	}
OUT:
	c.logger.Info().Msg("shutting down read loop")
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
			c.logger.Error().Msg("write queue was closed")
			goto OUT
		}
		c.logger.Info().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Bool("priority", priority).Msg("writing response for request")
		_, err := c.activeConn.Write(processRequest.ResponseBuffer.Bytes())
		if err != nil {
			c.logger.Error().Err(err).Msg("unable to write response")
			select {
			case <-c.ctx.Done():
				goto OUT
			case c.priorityWriteQueue <- processRequest:
				c.logger.Info().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Msg("requeueing response for priority writing")
			default:
				c.logger.Warn().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Msg("priority write queue is full, requeue of response for priority writing will block")
				c.priorityWriteQueue <- processRequest
			}
			goto OUT
		}
		polyglot.PutBuffer(processRequest.ResponseBuffer)
	}
OUT:
	c.logger.Info().Msg("shutting down write loop")
	c.cancel()
	c.wg.Done()
}

func (c *Client) worker(id int) {
	var processRequest *ProcessRequest
	var ok bool
	c.logger.Info().Int("worker", id).Msg("starting worker")
	for {
		select {
		case <-c.ctx.Done():
			goto OUT
		case processRequest, ok = <-c.processQueue:
		}
		if !ok {
			c.logger.Error().Int("worker", id).Msg("process queue was closed")
			goto OUT
		}
		c.logger.Info().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Int("worker", id).Msg("processing request")
		processRequest.Response.UUID = processRequest.Request.UUID
		c.handle(&processRequest.Request, &processRequest.Response)
		processRequest.ResponseBuffer = polyglot.GetBuffer()
		processRequest.Response.Encode(processRequest.ResponseBuffer)
		select {
		case c.writeQueue <- processRequest:
			c.logger.Info().Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Int("worker", id).Msg("queueing request for writing")
		default:
			c.logger.Warn().Int("worker", id).Str("uuid", processRequest.Request.UUID.String()).Uint32("type", processRequest.Request.Type).Msg("write queue is full, queuing request for writing will block")
			c.writeQueue <- processRequest
		}
	}
OUT:
	c.logger.Info().Int("worker", id).Msg("shutting down worker")
	c.workerWg.Done()
}

//func (c *Client) heartbeat() {
//	ticker := time.NewTicker(DefaultPingInterval)
//	defer ticker.Stop()
//	var err error
//	for {
//		select {
//		case <-c.closeCh:
//			c.wg.Done()
//			return
//		case <-ticker.C:
//			err = c.writePacket(PINGPacket, false)
//			if err != nil {
//				c.wg.Done()
//				_ = c.closeWithError(err)
//				return
//			}
//		}
//	}
//}
