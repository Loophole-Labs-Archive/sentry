// SPDX-License-Identifier: Apache-2.0

package listener

import (
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/loopholelabs/logging"
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
		logger:               options.Logger,
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
				lis.logger.Warnf("unable to close connection: %s", err)
			}
		}
		close(lis.availableConnections)
	}
	return nil
}

func (lis *Listener) accept() {
	for {
		conn, err := lis.listener.AcceptUnix()
		if err != nil {
			lis.logger.Errorf("unable to accept connection: %s", err)
			break
		}
		select {
		case lis.availableConnections <- conn:
		default:
			lis.logger.Warn("connection dropped")
		}
	}
	lis.wg.Done()
}
