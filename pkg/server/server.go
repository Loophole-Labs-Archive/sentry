// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"net"
	"sync"

	logging "github.com/loopholelabs/logging/types"

	"github.com/loopholelabs/sentry/pkg/listener"
	"github.com/loopholelabs/sentry/pkg/rpc"
)

var (
	OptionsErr = errors.New("invalid options")
	CreateErr  = errors.New("unable to create server")
	CloseErr   = errors.New("unable to close server")
)

type Server struct {
	listener   *listener.Listener
	activeConn *net.UnixConn

	ctx    context.Context
	cancel context.CancelFunc
	rpc    *rpc.Server

	logger logging.Logger
	wg     sync.WaitGroup
}

func New(options *Options) (*Server, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}
	lis, err := listener.New(options.listener())
	if err != nil {
		return nil, errors.Join(CreateErr, err)
	}

	s := &Server{
		listener: lis,
		logger:   options.Logger.SubLogger("server"),
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.rpc = rpc.NewServer(s.ctx, options.Logger)

	s.wg.Add(1)
	go s.handle()

	return s, nil
}

func (s *Server) Do(ctx context.Context, request *rpc.Request, response *rpc.Response) error {
	return s.rpc.Do(ctx, request, response)
}

func (s *Server) Close() error {
	err := s.listener.Close()
	if err != nil {
		return errors.Join(CloseErr, err)
	}
	s.cancel()
	s.wg.Wait()
	return nil
}

func (s *Server) handle() {
	var err error
	for {
		select {
		case <-s.ctx.Done():
			goto OUT
		default:
			s.logger.Info().Msg("waiting for connection")
			s.activeConn, err = s.listener.Accept()
			if err != nil {
				s.logger.Error().Err(err).Msg("unable to accept connection")
				goto OUT
			}
			s.logger.Info().Msg("connection was accepted")
			s.rpc.HandleConnection(s.activeConn)
		}
	}
OUT:
	s.logger.Info().Msg("shutting down handle")
	s.cancel()
	s.wg.Done()
}
