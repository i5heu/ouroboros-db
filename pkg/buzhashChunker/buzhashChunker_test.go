package buzhashChunker

import (
	"os"
	"testing"
)

func TestChunkBytes(t *testing.T) {
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

// This test will result in a average chunk size of 252.7 kB (kilobytes)
func TestChunkBytes_TestFile(t *testing.T) {
	// read bytes of file test.png
	file, err := os.Open("../../testData/img_v1.png")
	if err != nil {
		t.Errorf("Error opening file: %s", err)
	}
	fileInfo, err := file.Stat()
	if err != nil {
		t.Errorf("Error getting file info: %s", err)
	}
	fileSize := fileInfo.Size()
	fileBytes := make([]byte, fileSize)
	file.Read(fileBytes)

	// Call the function
	resultChan, _ := ChunkBytes(fileBytes)

	results := []ChunkInformation{}

	for result := range resultChan {
		results = append(results, result)
	}

	t.Log("Chunk number: ", len(results))

	size := 0

	for i, result := range results {
		t.Log("Chunk number: ", i)
		t.Log("Chunk len: ", len(result.Data))
		size += len(result.Data)
	}

	t.Log("Total size: source file: ", fileSize, " total size of chunks: ", size)
}
