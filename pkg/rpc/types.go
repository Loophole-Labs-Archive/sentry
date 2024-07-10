// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"errors"
	"github.com/google/uuid"
	"github.com/loopholelabs/polyglot/v2"
	"unsafe"
)

var (
	DecodeErr = errors.New("unable to decode buffer")
)

const (
	UUIDSize = unsafe.Sizeof(uuid.UUID{})
)

type Request struct {
	UUID [UUIDSize]byte
	Type uint32
	Data []byte
}

func (r *Request) Encode(buf *polyglot.Buffer) {
	if r.Data == nil {
		polyglot.Encoder(buf).Bytes(r.UUID[:]).Uint32(r.Type).Nil()
	} else {
		polyglot.Encoder(buf).Bytes(r.UUID[:]).Uint32(r.Type).Bytes(r.Data)
	}
}

func (r *Request) Decode(buf []byte) error {
	d := polyglot.Decoder(buf)
	var err error
	_, err = d.Bytes(r.UUID[:])
	if err != nil {
		return errors.Join(DecodeErr, err)
	}
	r.Type, err = d.Uint32()
	if err != nil {
		return errors.Join(DecodeErr, err)
	}
	if d.Nil() {
		r.Data = nil
		return nil
	}
	r.Data, err = d.Bytes(r.Data)
	if err != nil {
		return errors.Join(DecodeErr, err)
	}
	return nil
}

type Response struct {
	UUID  [UUIDSize]byte
	Error error
	Data  []byte
}

func (r *Response) Encode(buf *polyglot.Buffer) {
	if r.Error != nil {
		polyglot.Encoder(buf).Bytes(r.UUID[:]).Error(r.Error)
	} else if r.Data == nil {
		polyglot.Encoder(buf).Bytes(r.UUID[:]).Nil().Bytes(r.Data)
	}
}

func (r *Response) Decode(buf []byte) error {
	d := polyglot.Decoder(buf)
	var err error
	_, err = d.Bytes(r.UUID[:])
	if err != nil {
		return errors.Join(DecodeErr, err)
	}
	r.Error, err = d.Error()
	if err == nil {
		r.Data = nil
		return nil
	}
	if d.Nil() {
		r.Data, err = d.Bytes(r.Data)
		if err != nil {
			return errors.Join(DecodeErr, err)
		}
		return nil
	}
	return DecodeErr
}
