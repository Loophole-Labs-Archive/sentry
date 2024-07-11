// SPDX-License-Identifier: Apache-2.0

package client

import (
	"errors"
	"io"
	"time"

	"github.com/loopholelabs/logging"

	"github.com/loopholelabs/sentry/pkg/rpc"
)

type DialFunc func() (io.ReadWriteCloser, error)

const (
	maxBackoff = time.Second
	minBackoff = time.Millisecond * 5
)

var (
	OptionsErr = errors.New("invalid options")
)

type Client struct {
	rpc    *rpc.Client
	dial   DialFunc
	logger logging.Logger
}

func New(options *Options) (*Client, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}
	c := &Client{
		dial:   options.Dial,
		logger: options.Logger,
	}
	c.rpc = rpc.NewClient(options.Handle, options.Logger)
	go c.loop()
	return c, nil
}

func (c *Client) connect() io.ReadWriteCloser {
	var err error
	var backoff time.Duration
	var conn io.ReadWriteCloser
	for {
		conn, err = c.dial()
		if err == nil {
			return conn
		}
		c.logger.Errorf("unable to create connection: %v\n", err)
		if backoff == 0 {
			backoff = minBackoff
		} else if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
		c.logger.Infof("retrying in %s\n", backoff)
		time.Sleep(backoff)
	}
}

func (c *Client) loop() {
	for {
		c.logger.Info("creating connection\n")
		conn := c.connect()
		c.logger.Info("connection created\n")
		c.rpc.HandleConnection(conn)
	}
}
