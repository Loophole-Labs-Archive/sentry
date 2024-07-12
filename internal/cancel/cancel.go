// SPDX-License-Identifier: Apache-2.0

package cancel

import (
	"context"
	"errors"
	"sync"
)

var (
	CleanupErr = errors.New("unable to cleanup")
)

type CleanupFunc func() error

type Cancel struct {
	wg      sync.WaitGroup
	cancel  chan struct{}
	error   chan error
	cleanup CleanupFunc
}

func New(ctx context.Context, cleanup CleanupFunc) *Cancel {
	c := &Cancel{
		cancel:  make(chan struct{}),
		error:   make(chan error, 1),
		cleanup: cleanup,
	}
	c.wg.Add(1)
	go c.watch(ctx)
	return c
}

func (c *Cancel) Close() error {
	close(c.cancel)
	c.wg.Wait()
	return <-c.error
}

func (c *Cancel) watch(ctx context.Context) {
	select {
	case <-ctx.Done():
		err := c.cleanup()
		if err != nil {
			c.error <- errors.Join(CleanupErr, err)
			goto OUT
		}
		if err = ctx.Err(); err != nil {
			c.error <- errors.Join(context.Canceled, err)
			goto OUT
		}
		c.error <- context.Canceled
	case <-c.cancel:
		close(c.error)
	}
OUT:
	c.wg.Done()
}
