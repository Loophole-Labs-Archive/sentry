// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"errors"
	"github.com/loopholelabs/polyglot/v2"
)

var (
	UnknownErr = errors.New("unknown rpc type")
)

const (
	MaximumQueueSize          = 1024
	MaximumRequestPacketSize  = 1024
	MaximumResponsePacketSize = 1024
	NumWorkers                = 1
)

type HandleRPC func(*Request, *Response)

type Type uint32

const (
	TypePing Type = iota
)

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
