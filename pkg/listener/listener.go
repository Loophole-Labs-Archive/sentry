// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	logging "github.com/loopholelabs/logging/types"
)

var (
	OptionsErr = errors.New("invalid options")
	ListenErr  = errors.New("unable to listen")
	ClosedErr  = errors.New("listener closed")
	CloseErr   = errors.New("unable to close listener")
)

const (
	network = "unix"
)

const (
	stateListening = iota
	stateClosed
)

type Listener struct {
	listener             *net.UnixListener
	availableConnections chan *net.UnixConn
	state                atomic.Uint32
	logger               logging.Logger
	wg                   sync.WaitGroup
}

func New(options *Options) (*Listener, error) {
	if !validOptions(options) {
		return nil, OptionsErr
	}

	unixListener, err := net.ListenUnix(network, &net.UnixAddr{
		Name: options.UnixPath,
		Net:  network,
	})
	if err != nil {
		return nil, errors.Join(ListenErr, err)
	}

	lis := &Listener{
		listener:             unixListener,
		availableConnections: make(chan *net.UnixConn, options.MaxConn),
		logger:               options.Logger.SubLogger("listener"),
	}

	lis.state.Store(stateListening)
	lis.wg.Add(1)
	go lis.accept()

	return lis, nil
}

func (lis *Listener) Accept() (*net.UnixConn, error) {
	if lis.state.Load() == stateListening {
		conn, ok := <-lis.availableConnections
		if !ok {
			return nil, ClosedErr
		}
		return conn, nil
	}
	return nil, ClosedErr
}

func (lis *Listener) Close() error {
	if lis.state.CompareAndSwap(stateListening, stateClosed) {
		err := lis.listener.Close()
		if err != nil {
			return errors.Join(CloseErr, err)
		}
		lis.wg.Wait()
		for conn := range lis.availableConnections {
			err = conn.Close()
			if err != nil {
				lis.logger.Warn().Err(err).Msg("unable to close connection")
			}
		}
	}
	return nil
}

func (lis *Listener) accept() {
	for {
		conn, err := lis.listener.AcceptUnix()
		if err != nil {
			lis.logger.Error().Err(err).Msg("unable to accept connection")
			goto OUT
		}
		select {
		case lis.availableConnections <- conn:
		default:
			lis.logger.Warn().Msg("connection dropped")
		}
	}
OUT:
	close(lis.availableConnections)
	lis.wg.Done()
}
