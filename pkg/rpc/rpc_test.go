// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"github.com/loopholelabs/logging/types"
	"io"
	"math/rand"
	"net"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/loopholelabs/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func echoHandle(t *testing.T) HandleFunc {
	return func(req *Request, res *Response) {
		assert.Equal(t, req.UUID, res.UUID)
		assert.NoError(t, res.Error)
		res.Data = req.Data
	}
}

func TestRPCSimple(t *testing.T) {
	c1, c2 := net.Pipe()
	logger := logging.NewTest(t, logging.Zerolog, t.Name())
	logger.SetLevel(types.DebugLevel)
	ctx := context.Background()

	client := NewClient(echoHandle(t), logger)
	server := NewServer(ctx, logger)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		t.Logf("starting client")
		client.HandleConnection(c1)
		t.Logf("client exited")
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		t.Logf("starting server")
		server.HandleConnection(c2)
		t.Logf("server exited")
		wg.Done()
	}()

	req := Request{
		UUID: uuid.New(),
		Type: 32,
		Data: []byte("hello world"),
	}
	var res Response

	err := server.Do(context.Background(), &req, &res)
	require.NoError(t, err)

	assert.Equal(t, req.UUID, res.UUID)
	assert.NoError(t, res.Error)
	assert.Equal(t, req.Data, res.Data)

	err = c1.Close()
	require.NoError(t, err)
	err = c2.Close()
	require.NoError(t, err)

	wg.Wait()
}

func TestRPCSingleDisconnect(t *testing.T) {
	logger := logging.NewTest(t, logging.Zerolog, t.Name())
	logger.SetLevel(types.DebugLevel)
	ctx := context.Background()

	client := NewClient(echoHandle(t), logger)
	server := NewServer(ctx, logger)

	var wg sync.WaitGroup

	wg.Add(1)
	rpcDone := make(chan struct{})
	go func() {
		wg.Done()
		req := Request{
			UUID: uuid.New(),
			Type: 32,
			Data: []byte("hello world"),
		}
		var res Response
		err := server.Do(context.Background(), &req, &res)
		require.NoError(t, err)
		assert.Equal(t, req.UUID, res.UUID)
		assert.NoError(t, res.Error)
		assert.Equal(t, req.Data, res.Data)
		rpcDone <- struct{}{}
	}()
	wg.Wait()

	c1, c2 := net.Pipe()

	wg.Add(1)
	go func() {
		t.Logf("starting client")
		client.HandleConnection(c1)
		t.Logf("client exited")
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		t.Logf("starting server")
		server.HandleConnection(c2)
		t.Logf("server exited")
		wg.Done()
	}()

	timeout := time.NewTimer(100 * time.Millisecond).C
	select {
	case <-rpcDone:
	case <-timeout:
		t.Fatalf("timed out waiting for rpc to complete")
	}

	err := c1.Close()
	require.NoError(t, err)
	err = c2.Close()
	require.NoError(t, err)

	wg.Wait()
}

func TestRPCRandomDisconnects(t *testing.T) {
	const testTime = time.Second * 10
	logger := logging.NewTest(t, logging.Zerolog, t.Name())
	logger.SetLevel(types.DebugLevel)
	ctx := context.Background()

	client := NewClient(echoHandle(t), logger)
	server := NewServer(ctx, logger)

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	concurrentRPCs := make(chan struct{}, runtime.NumCPU())
	for i := 0; i < runtime.NumCPU(); i++ {
		concurrentRPCs <- struct{}{}
	}
	rpcDone := make(chan struct{})
	scheduledRPCs := uint32(0)
	var completedRPCs atomic.Uint32
	completedRPCs.Store(0)
	wg.Add(1)
	go func() {
		wg.Done()
		var rpcWg sync.WaitGroup
		for {
			select {
			case <-ctx.Done():
				t.Logf("waiting for rpcs to complete")
				rpcWg.Wait()
				close(rpcDone)
				return
			case <-concurrentRPCs:
				rpcWg.Add(1)
				go func(i uint32) {
					req := Request{
						UUID: uuid.New(),
						Type: i,
						Data: []byte("hello world"),
					}
					var res Response
					err := server.Do(ctx, &req, &res)
					if err != nil {
						select {
						case <-ctx.Done():
							goto OUT
						default:
							t.Errorf("error in rpc: %v", err)
						}
					}
					require.Equal(t, req.UUID, res.UUID)
					require.NoError(t, res.Error)
					require.Equal(t, req.Data, res.Data)
					completedRPCs.Add(1)
				OUT:
					concurrentRPCs <- struct{}{}
					rpcWg.Done()
				}(scheduledRPCs)
				scheduledRPCs++
			}
		}
	}()
	wg.Wait()

	startClient := func(conn io.ReadWriteCloser) {
		wg.Add(1)
		go func() {
			t.Logf("starting client")
			client.HandleConnection(conn)
			t.Logf("client exited")
			wg.Done()
		}()
	}
	startServer := func(conn io.ReadWriteCloser) {
		wg.Add(1)
		go func() {
			t.Logf("starting server")
			server.HandleConnection(conn)
			t.Logf("server exited")
			wg.Done()
		}()
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(done)
				return
			default:
				_t := time.Duration(rand.Intn(100)) * time.Millisecond
				t.Logf("will start client and server connections in %v", _t)
				time.Sleep(_t)
				t.Logf("starting client and server")
				c1, c2 := net.Pipe()
				startClient(c1)
				startServer(c2)
				_t = time.Duration(rand.Intn(100)) * time.Millisecond
				t.Logf("will kill client and server connections in %v", _t)
				time.Sleep(_t)
				t.Logf("killing client and server connections")
				err := c1.Close()
				require.NoError(t, err)
				err = c2.Close()
				require.NoError(t, err)
				wg.Wait()
				t.Logf("killed client and server connections")
			}
		}
	}()

	t.Logf("running tests for %v", testTime)
	time.Sleep(testTime)

	cancel()
	timeout := time.NewTimer(500 * time.Millisecond).C
	select {
	case <-done:
	case <-timeout:
		t.Fatalf("timed out waiting for test runner to exit")
	}

	timeout = time.NewTimer(500 * time.Millisecond).C
	select {
	case <-rpcDone:
	case <-timeout:
		t.Fatalf("timed out waiting for rpcs to complete")
	}

	t.Logf("%d rpcs were scheduled", scheduledRPCs)
	t.Logf("%d rpc completed successfully", completedRPCs.Load())
}
