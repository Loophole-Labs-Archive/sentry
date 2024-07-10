// SPDX-License-Identifier: Apache-2.0

package rpc

import (
	"crypto/rand"
	"github.com/google/uuid"
	"github.com/loopholelabs/polyglot/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
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
			UUID: [UUIDSize]byte(_uuid[:]),
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
			UUID: [UUIDSize]byte(_uuid[:]),
			Type: 32,
			Data: nil,
		}
		encoded.Encode(buf)
		assert.Equal(t, 22, buf.Len())

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
