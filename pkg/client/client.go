// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"io"
	"time"

	"github.com/loopholelabs/logging"

	"github.com/loopholelabs/sentry/internal/vsock"
	"github.com/loopholelabs/sentry/pkg/rpc"
)

const (
	maxBackoff = time.Second
	minBackoff = time.Millisecond * 5
)

var (
	OptionsErr = errors.New("invalid options")
)

type Client struct {
	rpc    *rpc.Client
	cid    uint32
	port   uint32
	logger logging.Logger
}

func New(options *Options) (*Client, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}
	c := &Client{
		rpc:    rpc.NewClient(options.Logger),
		cid:    options.CID,
		port:   options.Port,
		logger: options.Logger,
	}
	go c.handle()
	return c, nil
}

func (c *Client) connect() io.ReadWriteCloser {
	var err error
	var backoff time.Duration
	var conn io.ReadWriteCloser
	for {
		conn, err = vsock.Dial(c.cid, c.port)
		if err == nil {
			return conn
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
		c.logger.Infof("retrying in %s", backoff)
		time.Sleep(backoff)
	}
}

func (c *Client) handle() {
	for {
		c.logger.Info("creating connection")
		conn := c.connect()
		c.logger.Info("connection created")
		c.rpc.HandleConnection(conn)
	}
}
