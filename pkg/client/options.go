// SPDX-License-Identifier: Apache-2.0

package client

import (
	"github.com/loopholelabs/logging"
)

type Options struct {
	Dial   DialFunc
	Logger logging.Logger
}

func validOptions(options *Options) bool {
	return options != nil && options.Dial != nil && options.Logger != nil
}
