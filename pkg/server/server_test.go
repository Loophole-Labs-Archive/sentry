// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/loopholelabs/sentry/pkg/client"
	"github.com/loopholelabs/sentry/pkg/rpc"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"testing"

	"github.com/loopholelabs/logging"
	"github.com/stretchr/testify/require"
)

func testDialFunc(path string) client.DialFunc {
	return func() (io.ReadWriteCloser, error) {
		return net.DialUnix("unix", nil, &net.UnixAddr{
			Name: path,
			Net:  "unix",
		})
	}
}

func echoHandle(t *testing.T) rpc.HandleFunc {
	return func(req *rpc.Request, res *rpc.Response) {
		assert.Equal(t, req.UUID, res.UUID)
		assert.NoError(t, res.Error)
		res.Data = req.Data
	}
}

func TestReconnect(t *testing.T) {
	logger := logging.NewTestLogger(t)
	serverOpts := &Options{
		UnixPath: fmt.Sprintf("%s/%s.sock", t.TempDir(), t.Name()),
		MaxConn:  1,
		Logger:   logger,
	}

	clientOpts := &client.Options{
		Handle: echoHandle(t),
		Dial:   testDialFunc(serverOpts.UnixPath),
		Logger: logger,
	}

	s, err := New(serverOpts)
	require.NoError(t, err)

	c, err := client.New(clientOpts)
	require.NoError(t, err)

	req := rpc.Request{
		UUID: uuid.New(),
		Type: 32,
		Data: []byte("hello world"),
	}

	var res rpc.Response

	err = s.Do(context.Background(), &req, &res)
	require.NoError(t, err)
	assert.Equal(t, req.UUID, res.UUID)
	assert.NoError(t, res.Error)
	assert.Equal(t, req.Data, res.Data)

	err = s.Close()
	require.NoError(t, err)

	s, err = New(serverOpts)
	require.NoError(t, err)

	err = s.Do(context.Background(), &req, &res)
	require.NoError(t, err)
	assert.Equal(t, req.UUID, res.UUID)
	assert.NoError(t, res.Error)
	assert.Equal(t, req.Data, res.Data)

	err = s.Close()
	require.NoError(t, err)

	err = c.Close()
	require.NoError(t, err)
}
