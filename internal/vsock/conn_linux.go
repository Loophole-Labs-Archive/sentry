//go:build linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"errors"
	"io"
	"sync/atomic"

	"golang.org/x/sys/unix"
)

var (
	BadFDErr = errors.New("bad file descriptor")
	ReadErr  = errors.New("unable to read from vsock connection")
	WriteErr = errors.New("unable to write to vsock connection")
	CloseErr = errors.New("unable to close vsock connection")
)

const (
	stateConnected = iota
	stateClosed
)

type conn struct {
	state atomic.Uint32
	fd    int
}

func newConn(fd int) *conn {
	return &conn{
		fd: fd,
	}
}

func (c *conn) Read(b []byte) (int, error) {
	n, err := unix.Read(c.fd, b)
	if err != nil {
		if errors.Is(err, unix.EBADF) {
			err = errors.Join(BadFDErr, err)
		}
		return n, errors.Join(ReadErr, err)
	}
	if n == 0 {
		return 0, errors.Join(ReadErr, io.EOF)
	}
	return n, nil
}

func (c *conn) Write(b []byte) (int, error) {
	n, err := unix.Write(c.fd, b)
	if err != nil {
		if errors.Is(err, unix.EBADF) {
			err = errors.Join(BadFDErr, err)
		}
		return n, errors.Join(WriteErr, err)
	}
	if n == 0 {
		return 0, errors.Join(WriteErr, io.EOF)
	}
	return n, nil
}

func (c *conn) Close() error {
	if c.state.CompareAndSwap(stateConnected, stateClosed) {
		if err := unix.Shutdown(c.fd, unix.SHUT_RDWR); err != nil {
			if _err := unix.Close(c.fd); _err != nil {
				err = errors.Join(err, _err)
			}
			return errors.Join(CloseErr, err)
		}
		if err := unix.Close(c.fd); err != nil {
			return errors.Join(CloseErr, err)
		}
	}
	return nil
}
