package buzhashChunker

import (
	"testing"
)

func TestChunkBytes(t *testing.T) {
	// Initialize variables for testing
	var input []byte = []byte("Hello World")
	expectedChunk := ChunkInformation{
		ChunkNumber: 0,
		Data:        []byte("Hello World"),
	}

	// Call the function
	resultChan, _ := ChunkBytes(input)
	result := <-resultChan

	// Check the result
	if result.ChunkNumber != expectedChunk.ChunkNumber {
		t.Errorf("Expected chunk number %d, got %d", expectedChunk.ChunkNumber, result.ChunkNumber)
	}

	if string(result.Data) != string(expectedChunk.Data) {
		t.Errorf("Expected data %s, got %s", string(expectedChunk.Data), string(result.Data))
	}
}
