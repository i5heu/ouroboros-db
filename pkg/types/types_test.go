package types_test

import (
	"bytes"
	"crypto/sha512"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestEvent_CreateDetailsMetaHash(t *testing.T) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventType: types.Normal,
			FastMeta:  types.FastMeta{[]byte("fast meta")},
		},
		Level:          42,
		Content:        types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("hello world 3")), DataLength: 10}},
		Metadata:       types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("hello world 5")), DataLength: 10}},
		ParentEvent:    sha512.Sum512([]byte("hello world 7")),
		RootEvent:      sha512.Sum512([]byte("hello world 8")),
		Temporary:      types.Binary(true),
		FullTextSearch: types.Binary(true),
	}

	expectedHash := event.CreateDetailsMetaHash()

	// Manually create the expected hash for comparison
	var buffer bytes.Buffer

	buffer.WriteString(event.EventType.String())
	buffer.Write(event.Level.Bytes())
	buffer.Write(event.FastMeta.Hash().Bytes())
	buffer.Write(event.Metadata.Hash().Bytes())
	buffer.Write(event.Content.Hash().Bytes())
	buffer.Write(event.ParentEvent.Bytes())
	buffer.Write(event.RootEvent.Bytes())
	buffer.Write(event.Temporary.Bytes())
	buffer.Write(event.FullTextSearch.Bytes())

	manualHash := sha512.Sum512(buffer.Bytes())

	assert.Equal(t, types.Hash(manualHash), expectedHash)
}

func TestEvent_Key(t *testing.T) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventHash: sha512.Sum512([]byte("hello world 2")),
			EventType: types.Normal,
			FastMeta:  types.FastMeta{[]byte("fast meta")},
		},
	}

	expectedKey := append([]byte("Event:"), event.EventType.Bytes()...)
	expectedKey = append(expectedKey, event.EventHash.Bytes()...)

	fastMetaBytes := event.FastMeta.Bytes()
	expectedKey = append(expectedKey, fastMetaBytes...)

	assert.Equal(t, types.EventKeyBytes(expectedKey), event.Key())
}

func TestEventKeyBytes_Deserialize(t *testing.T) {
	// Helper function to create valid EventKeyBytes
	validKeyBytes := func() types.EventKeyBytes {
		event := types.Event{
			EventIdentifier: types.EventIdentifier{
				EventHash: sha512.Sum512([]byte("hello world 2")),
				EventType: types.Normal,
				FastMeta:  types.FastMeta{[]byte("fast meta")},
			},
		}
		return event.Key()
	}()

	// Deserialize the valid key
	deserializedEventIdentifier, err := validKeyBytes.Deserialize()
	assert.NoError(t, err)
	assert.NotNil(t, deserializedEventIdentifier)
	assert.Equal(t, types.Normal, deserializedEventIdentifier.EventType)
	assert.Equal(t, types.Hash(sha512.Sum512([]byte("hello world 2"))), deserializedEventIdentifier.EventHash)
	assert.Equal(t, types.FastMeta{[]byte("fast meta")}, deserializedEventIdentifier.FastMeta)

	// Test with invalid key bytes (missing fastMeta)
	invalidKeyBytes := []byte("Event:0" + string(make([]byte, 63)))
	_, err = types.EventKeyBytes(invalidKeyBytes).Deserialize()
	assert.Error(t, err, "Expected an error for invalid EventKeyBytes format")

	// Test with short key bytes
	shortKeyBytes := []byte("Event:0" + string(make([]byte, 63)))
	_, err = types.EventKeyBytes(shortKeyBytes).Deserialize()
	assert.Error(t, err, "Expected an error for short EventKeyBytes")
}

func TestEventKeyBytes_DeserializeInvalid(t *testing.T) {
	invalidKeyBytes := []byte("Invalid:Key:Bytes")
	_, err := types.EventKeyBytes(invalidKeyBytes).Deserialize()
	assert.Error(t, err, "Expected an error for invalid EventKeyBytes format")
}

func TestChunkMetaCollection_Hash(t *testing.T) {
	chunkMeta1 := types.ChunkMeta{Hash: sha512.Sum512([]byte("chunk1")), DataLength: 10}
	chunkMeta2 := types.ChunkMeta{Hash: sha512.Sum512([]byte("chunk2")), DataLength: 20}
	chunkMetaCollection := types.ChunkMetaCollection{chunkMeta1, chunkMeta2}

	expectedHash := sha512.Sum512(append(chunkMeta1.Hash.Bytes(), chunkMeta2.Hash.Bytes()...))

	assert.Equal(t, types.Hash(expectedHash), chunkMetaCollection.Hash())
}
