// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"github.com/loopholelabs/sentry/internal/vsock"
	"io"
	"sync"
	"time"

	"github.com/loopholelabs/logging"
)

const (
	maxBackoff = time.Second
	minBackoff = time.Millisecond * 5
)

var (
	OptionsErr = errors.New("invalid options")
)

type Client struct {
	activeConn io.ReadWriteCloser
	cid        uint32
	port       uint32
	logger     logging.Logger
	wg         sync.WaitGroup
}

func New(options *Options) (*Client, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}

	c := &Client{
		cid:    options.CID,
		port:   options.Port,
		logger: options.Logger,
	}
	c.wg.Add(1)
	go c.read()
	return c, nil
}

func (c *Client) connect() {
	var err error
	var backoff time.Duration
	for {
		c.activeConn, err = vsock.Dial(c.cid, c.port)
		if err == nil {
			break
		}
		c.logger.Errorf("unable to create connection: %v", err)
		if backoff == 0 {
			backoff = minBackoff
		} else if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
		c.logger.Infof("retrying in %v", backoff)
		time.Sleep(backoff)
	}
}

func (c *Client) read() {
	buf := make([]byte, 4096)
CONNECT:
	if c.activeConn == nil {
		c.logger.Info("creating connection")
		c.connect()
		c.logger.Info("connection created")
	}
	for {
		n, err := c.activeConn.Read(buf)
		if err != nil {
			c.logger.Errorf("unable to read from connection: %v", err)
			_ = c.activeConn.Close()
			c.activeConn = nil
			goto CONNECT
		}
		c.logger.Infof("read %d bytes", n)
	}

	c.wg.Done()
}
