//go:build !linux

// SPDX-License-Identifier: Apache-2.0

package vsock

import (
	"errors"

	"github.com/loopholelabs/sentry/pkg/client"
)

var (
	UnsupportedErr = errors.New("not supported on this platform")
)

func DialFunc(uint32, uint32) (client.DialFunc, error) {
	return nil, UnsupportedErr
}
