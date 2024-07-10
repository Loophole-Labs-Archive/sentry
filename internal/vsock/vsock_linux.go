//go:build linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"errors"
	"io"

	"golang.org/x/sys/unix"
)

var (
	CreationErr   = errors.New("unable to create vsock connection")
	ConnectionErr = errors.New("unable to connect to vsock")
)

func Dial(cid uint32, port uint32) (io.ReadWriteCloser, error) {
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
