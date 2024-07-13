// SPDX-License-Identifier: Apache-2.0

package cancel

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var (
	CleanupErr = errors.New("unable to cleanup")
)

const (
	StateWatching = iota
	StateClosed
)

type CleanupFunc func() error

type Cancel struct {
	wg       sync.WaitGroup
	cancel   chan struct{}
	error    chan error
	cleanup  CleanupFunc
	state    atomic.Uint32
	closeErr error
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
	if c.state.CompareAndSwap(StateWatching, StateClosed) {
		close(c.cancel)
		c.wg.Wait()
		c.closeErr = <-c.error
	}
	return c.closeErr
}

func (c *Cancel) CloseIgnoreError() {
	_ = c.Close()
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
