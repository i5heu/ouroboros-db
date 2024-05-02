package buzhashChunker

import (
	"crypto/sha512"
	"testing"
)

func TestChunkBytes(t *testing.T) {
	// Initialize variables for testing
	var input []byte = []byte("Hello World")
	var expectedOutput []ChunkData = []ChunkData{
		{
			Hash: sha512.Sum512([]byte("Hello World")),
			Data: []byte("Hello World"),
		},
	}

	// Call the function with the test input
	output, err := ChunkBytes(input)
	if err != nil {
		t.Errorf("ChunkBytes(%s) returned an error: %v", input, err)
	}

	// Check the function's output against the expected output
	if len(output) != len(expectedOutput) {
		t.Errorf("ChunkBytes(%s) returned %d chunks, expected %d", input, len(output), len(expectedOutput))
	}

	for i, chunk := range output {
		if string(chunk.Data) != string(expectedOutput[i].Data) {
			t.Errorf("ChunkBytes(%s)[%d].Data = %s, expected %s", input, i, chunk.Data, expectedOutput[i].Data)
		}
	}
}
