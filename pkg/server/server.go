// SPDX-License-Identifier: Apache-2.0

package server

import (
	"errors"

	"github.com/loopholelabs/sentry/internal/listener"
)

var (
	OptionsErr = errors.New("invalid options")
	CreateErr  = errors.New("unable to create server")
	CloseErr   = errors.New("unable to close server")
)

type Server struct {
	listener *listener.Listener
}

func New(options *Options) (*Server, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}
	lis, err := listener.New(options.listener())
	if err != nil {
		return nil, errors.Join(CreateErr, err)
	}
	return &Server{
		listener: lis,
	}, nil
}

func (s *Server) Close() error {
	err := s.listener.Close()
	if err != nil {
		return errors.Join(CloseErr, err)
	}
	return nil
}
