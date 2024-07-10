// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"crypto/rand"
	"testing"

	"github.com/google/uuid"
	"github.com/loopholelabs/polyglot/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest(t *testing.T) {
	buf := polyglot.GetBuffer()
	t.Cleanup(func() {
		polyglot.PutBuffer(buf)
	})

	_uuid := uuid.New()

	expectedData := make([]byte, 128)
	_, err := rand.Read(expectedData)
	require.NoError(t, err)

	t.Run("Simple", func(t *testing.T) {
		encoded := Request{
			UUID: _uuid,
			Type: 32,
			Data: expectedData,
		}
		encoded.Encode(buf)
		assert.Equal(t, 153, buf.Len())

		var decoded Request
		err = decoded.Decode(buf.Bytes())
		require.NoError(t, err)

		assert.Equal(t, encoded.UUID, decoded.UUID)
		assert.Equal(t, encoded.Type, decoded.Type)
		assert.Equal(t, encoded.Data, decoded.Data)

		buf.Reset()
	})

	t.Run("NilData", func(t *testing.T) {
		encoded := Request{
			UUID: _uuid,
			Type: 32,
			Data: nil,
		}
		encoded.Encode(buf)
		assert.Equal(t, MinimumRequestSize, buf.Len())

		decoded := Request{
			Data: expectedData,
		}
		err = decoded.Decode(buf.Bytes())
		require.NoError(t, err)

		assert.Equal(t, encoded.UUID, decoded.UUID)
		assert.Equal(t, encoded.Type, decoded.Type)
		assert.Nil(t, decoded.Data)

		buf.Reset()
	})
}

func TestResponse(t *testing.T) {
	buf := polyglot.GetBuffer()
	t.Cleanup(func() {
		polyglot.PutBuffer(buf)
	})

	_uuid := uuid.New()

	randomData := make([]byte, 128)
	_, err := rand.Read(randomData)
	require.NoError(t, err)

	t.Run("NoError", func(t *testing.T) {
		encoded := Response{
			UUID:  _uuid,
			Error: nil,
			Data:  randomData,
		}
		encoded.Encode(buf)
		assert.Equal(t, 152, buf.Len())

		var decoded Response
		err = decoded.Decode(buf.Bytes())
		require.NoError(t, err)

		assert.Equal(t, encoded.UUID, decoded.UUID)
		assert.Nil(t, decoded.Error)
		assert.Equal(t, encoded.Data, decoded.Data)

		buf.Reset()
	})

	t.Run("NilData", func(t *testing.T) {
		encoded := Response{
			UUID:  _uuid,
			Error: nil,
			Data:  nil,
		}
		encoded.Encode(buf)
		assert.Equal(t, 21, buf.Len())

		var decoded Response
		err = decoded.Decode(buf.Bytes())
		require.NoError(t, err)

		assert.Equal(t, encoded.UUID, decoded.UUID)
		assert.Nil(t, decoded.Error)
		assert.Nil(t, decoded.Data)

		buf.Reset()
	})

	t.Run("Error", func(t *testing.T) {
		encoded := Response{
			UUID:  _uuid,
			Error: assert.AnError,
			Data:  randomData,
		}
		encoded.Encode(buf)
		assert.Equal(t, 63, buf.Len())

		var decoded Response
		err = decoded.Decode(buf.Bytes())
		require.NoError(t, err)

		assert.Equal(t, encoded.UUID, decoded.UUID)
		assert.EqualError(t, encoded.Error, decoded.Error.Error())
		assert.Nil(t, decoded.Data)

		buf.Reset()
	})
}
