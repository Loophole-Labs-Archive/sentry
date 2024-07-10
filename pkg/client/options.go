// SPDX-License-Identifier: Apache-2.0

package client

import "github.com/loopholelabs/logging"

type Options struct {
	CID    uint32
	Port   uint32
	Logger logging.Logger
}

func validOptions(options *Options) bool {
	return options != nil && options.Logger != nil
}
