//go:build linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"context"
	"errors"
	"io"
	"sync"

	"golang.org/x/sys/unix"

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
		return dialContext(ctx, cid, port)
	}, nil
}

func dialContext(ctx context.Context, cid uint32, port uint32) (io.ReadWriteCloser, error) {
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, errors.Join(CreationErr, err)
	}
	var wg sync.WaitGroup
	cancel := make(chan struct{})
	cancelError := make(chan error, 1)
	wg.Add(1)
	go func() {
		select {
		case <-ctx.Done():
			if err := unix.Shutdown(fd, unix.SHUT_RDWR); err != nil {
				if e := unix.Close(fd); e != nil {
					err = errors.Join(CloseErr, e, err)
				}
				cancelError <- errors.Join(ShutdownErr, err)
				goto OUT
			}
			if err := unix.Close(fd); err != nil {
				cancelError <- errors.Join(CloseErr, err)
				goto OUT
			}
			if err := ctx.Err(); err != nil {
				cancelError <- errors.Join(context.Canceled, err)
				goto OUT
			}
			cancelError <- context.Canceled
		case <-cancel:
			close(cancelError)
		}
	OUT:
		wg.Done()
	}()
	if err = unix.Connect(fd, &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}); err != nil {
		return nil, errors.Join(ConnectionErr, err)
	}
	close(cancel)
	wg.Wait()
	err, ok := <-cancelError
	if ok {
		return nil, err
	}
	return newConn(fd), nil
}
