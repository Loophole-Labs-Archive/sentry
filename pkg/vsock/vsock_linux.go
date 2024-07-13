//go:build linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"context"
	"errors"
	"io"

	"golang.org/x/sys/unix"

	"github.com/loopholelabs/sentry/internal/cancel"
	"github.com/loopholelabs/sentry/pkg/client"
)

var (
	CreationErr   = errors.New("unable to create vsock connection")
	ConnectionErr = errors.New("unable to connect to vsock")
	ShutdownErr   = errors.New("unable to shutdown vsock connection")
	CloseErr      = errors.New("unable to close vsock connection")
)

func DialFunc(cid uint32, port uint32) (client.DialFunc, error) {
	return func(ctx context.Context) (io.ReadWriteCloser, error) {
		return DialContext(ctx, cid, port)
	}, nil
}

func DialContext(ctx context.Context, cid uint32, port uint32) (io.ReadWriteCloser, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, errors.Join(CreationErr, err)
	}
	c := cancel.New(ctx, cleanup(fd))
	defer c.CloseIgnoreError()
	if err = unix.Connect(fd, &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}); err != nil {
		return nil, errors.Join(ConnectionErr, err)
	}
	err = c.Close()
	if err != nil {
		return nil, err
	}
	return newConn(fd), nil
}

func cleanup(fd int) cancel.CleanupFunc {
	return func() error {
		if err := unix.Shutdown(fd, unix.SHUT_RDWR); err != nil {
			if e := unix.Close(fd); e != nil {
				err = errors.Join(CloseErr, e, err)
			}
			return errors.Join(ShutdownErr, err)
		}
		if err := unix.Close(fd); err != nil {
			return errors.Join(CloseErr, err)
		}
		return nil
	}
}
