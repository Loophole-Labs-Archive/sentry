//go:build linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"errors"
	"io"

	"golang.org/x/sys/unix"

	"github.com/loopholelabs/sentry/pkg/client"
)

var (
	CreationErr   = errors.New("unable to create vsock connection")
	ConnectionErr = errors.New("unable to connect to vsock")
)

func DialFunc(cid uint32, port uint32) (client.DialFunc, error) {
	return func() (io.ReadWriteCloser, error) {
		return dial(cid, port)
	}, nil
}

func dial(cid uint32, port uint32) (io.ReadWriteCloser, error) {
	// TODO: convert to dialContext, drafter has the implementation in the internal/vsock package
	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, errors.Join(CreationErr, err)
	}
	if err = unix.Connect(fd, &unix.SockaddrVM{
		CID:  cid,
		Port: port,
	}); err != nil {
		return nil, errors.Join(ConnectionErr, err)
	}
	return newConn(fd), nil
}
