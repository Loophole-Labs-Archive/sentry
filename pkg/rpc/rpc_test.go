// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/loopholelabs/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net"
	"sync"
	"testing"
)

func echoHandle(t *testing.T) HandleRPC {
	return func(req *Request, res *Response) {
		t.Logf("handling request %v", req)
		assert.Equal(t, req.UUID, res.UUID)
		assert.NoError(t, res.Error)
		res.Data = req.Data
	}
}

func TestRPCSimple(t *testing.T) {
	c1, c2 := net.Pipe()
	logger := logging.NewTestLogger(t)
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

	err := server.DoRPC(&req, &res)
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
