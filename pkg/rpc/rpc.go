// SPDX-License-Identifier: Apache-2.0

package rpc

import "github.com/loopholelabs/polyglot/v2"

const (
	MaximumQueueSize          = 1024
	MaximumRequestPacketSize  = 1024
	MaximumResponsePacketSize = 1024
	NumWorkers                = 1
)

type HandleFunc func(*Request, *Response)

type ProcessRequest struct {
	Request        Request
	Response       Response
	ResponseBuffer *polyglot.Buffer
}

type InflightRequest struct {
	Request       *Request
	Response      *Response
	RequestBuffer *polyglot.Buffer
	complete      chan struct{}
}
