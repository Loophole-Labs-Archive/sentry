// SPDX-License-Identifier: Apache-2.0

package server

import (
	logging "github.com/loopholelabs/logging/types"
	"github.com/loopholelabs/sentry/pkg/listener"
)

type Options struct {
	UnixPath string
	MaxConn  int
	Logger   logging.Logger
}

func validOptions(options *Options) bool {
	return options != nil && options.UnixPath != "" && options.MaxConn > 0 && options.Logger != nil
}

func (options *Options) listener() *listener.Options {
	return &listener.Options{
		UnixPath: options.UnixPath,
		MaxConn:  options.MaxConn,
		Logger:   options.Logger,
	}
}
