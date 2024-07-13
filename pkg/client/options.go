// SPDX-License-Identifier: Apache-2.0

package client

import (
	logging "github.com/loopholelabs/logging/types"
	"github.com/loopholelabs/sentry/pkg/rpc"
)

type Options struct {
	Handle rpc.HandleFunc
	Dial   DialFunc
	Logger logging.SubLogger
}

func validOptions(options *Options) bool {
	return options != nil && options.Dial != nil && options.Handle != nil && options.Logger != nil
}
