// SPDX-License-Identifier: Apache-2.0

package listener

import (
	logging "github.com/loopholelabs/logging/types"
)

type Options struct {
	UnixPath string
	MaxConn  int
	Logger   logging.SubLogger
}

func validOptions(options *Options) bool {
	return options != nil && options.UnixPath != "" && options.MaxConn > 0 && options.Logger != nil
}
