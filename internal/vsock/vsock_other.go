//go:build !linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"context"
	"errors"
	"io"
)

var (
	UnsupportedErr = errors.New("not supported on this platform")
)

func DialContext(context.Context, uint32, uint32) (io.ReadWriteCloser, error) {
	return nil, UnsupportedErr
}
