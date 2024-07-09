// SPDX-License-Identifier: Apache-2.0

package listener

import "github.com/loopholelabs/logging"

type Options struct {
	UnixPath string
	MaxConn  int
	Logger   logging.Logger
}

func validOptions(options *Options) bool {
	return options != nil && options.UnixPath != "" && options.MaxConn > 0 && options.Logger != nil
}
