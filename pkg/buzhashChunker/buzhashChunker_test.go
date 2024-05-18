package buzhashChunker

import (
	"crypto/sha512"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/types"
)

func TestChunkBytes(t *testing.T) {
	// Initialize variables for testing
	var input []byte = []byte("Hello World")
	expectedHash := sha512.Sum512([]byte("Hello World"))
	var expectedChunk types.Chunk = types.Chunk{
		ChunkMeta: types.ChunkMeta{
			Hash:       expectedHash,
			DataLength: uint32(len(input)),
		},
		Data: input,
	}
	var expectedChunks types.ChunkCollection = types.ChunkCollection{expectedChunk}
	var expectedMeta types.ChunkMetaCollection = types.ChunkMetaCollection{expectedChunk.ChunkMeta}

	// Call the function with the test input
	chunks, meta, err := ChunkBytes(input)
	if err != nil {
		t.Errorf("ChunkBytes(%s) returned an error: %v", input, err)
	}

	// Check the function's output against the expected output
	if len(chunks) != len(expectedChunks) {
		t.Errorf("ChunkBytes(%s) returned %d chunks, expected %d", input, len(chunks), len(expectedChunks))
	}

	if len(meta) != len(expectedMeta) {
		t.Errorf("ChunkBytes(%s) returned %d metadata chunks, expected %d", input, len(meta), len(expectedMeta))
	}

	for i, chunk := range chunks {
		if chunk.Hash != expectedChunks[i].Hash {
			t.Errorf("ChunkBytes(%s)[%d].Hash = %x, expected %x", input, i, chunk.Hash, expectedChunks[i].Hash)
		}
		if string(chunk.Data) != string(expectedChunks[i].Data) {
			t.Errorf("ChunkBytes(%s)[%d].Data = %s, expected %s", input, i, chunk.Data, expectedChunks[i].Data)
		}
	}

	for i, metaChunk := range meta {
		if metaChunk.Hash != expectedMeta[i].Hash {
			t.Errorf("ChunkBytes(%s) Metadata[%d].Hash = %x, expected %x", input, i, metaChunk.Hash, expectedMeta[i].Hash)
		}
		if metaChunk.DataLength != expectedMeta[i].DataLength {
			t.Errorf("ChunkBytes(%s) Metadata[%d].DataLength = %d, expected %d", input, i, metaChunk.DataLength, expectedMeta[i].DataLength)
		}
	}
}
