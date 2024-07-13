// SPDX-License-Identifier: Apache-2.0

package cancel

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"testing"
	"time"
)

var (
	testCleanupErr = errors.New("test cleanup error")
)

func errCleanupFunc(called *bool) CleanupFunc {
	return func() error {
		*called = true
		return testCleanupErr
	}
}

func nilCleanupFunc(called *bool) CleanupFunc {
	return func() error {
		*called = true
		return nil
	}
}

func TestCancelCloseError(t *testing.T) {
	defer goleak.VerifyNone(t)

	called := false
	c := New(context.Background(), errCleanupFunc(&called))
	defer c.CloseIgnoreError()

	time.Sleep(time.Millisecond * 50)

	require.False(t, called)

	err := c.Close()
	require.NoError(t, err)
	require.False(t, called)
}

func TestCancelCloseNil(t *testing.T) {
	defer goleak.VerifyNone(t)

	called := false
	c := New(context.Background(), nilCleanupFunc(&called))
	defer c.CloseIgnoreError()

	time.Sleep(time.Millisecond * 50)

	require.False(t, called)

	err := c.Close()
	require.NoError(t, err)
	require.False(t, called)
}

func TestCancelContextErr(t *testing.T) {
	defer goleak.VerifyNone(t)

	called := false
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, errCleanupFunc(&called))
	defer c.CloseIgnoreError()

	time.Sleep(time.Millisecond * 50)

	require.False(t, called)
	cancel()

	err := c.Close()
	require.ErrorIs(t, err, testCleanupErr)
	require.True(t, called)
}

func TestCancelContextNil(t *testing.T) {
	defer goleak.VerifyNone(t)

	called := false
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, nilCleanupFunc(&called))
	defer c.CloseIgnoreError()

	time.Sleep(time.Millisecond * 50)

	require.False(t, called)
	cancel()

	err := c.Close()
	require.ErrorIs(t, err, context.Canceled)
	require.True(t, called)
}

func TestCancelDefer(t *testing.T) {
	defer goleak.VerifyNone(t)

	called := false
	ctx, cancel := context.WithCancel(context.Background())
	c := New(ctx, nilCleanupFunc(&called))

	time.Sleep(time.Millisecond * 50)

	require.False(t, called)

	c.CloseIgnoreError()

	require.False(t, called)
	cancel()
	err := c.Close()
	require.NoError(t, err)
	require.False(t, called)
}
