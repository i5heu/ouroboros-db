package chunker

import (
	"os"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/types"
)

func TestChunkBytes(t *testing.T) {
	var input []byte = []byte("Hello World")
	expectedChunk := types.ECChunk{
		ECCMeta: types.ECCMeta{
			BuzChunkNumber: uint32(0),
		},
		EncryptedData: []byte{0x5c, 0xf8, 0x06, 0x58, 0x17, 0xd1, 0xb2, 0xcc, 0x1b, 0x8a, 0x3c, 0x69, 0x3f, 0x40, 0x00, 0x0c, 0xad, 0x3e, 0x03, 0x72, 0x47, 0x3b},
	}

	// Call the function
	resultChan, _ := ChunkBytes(input, 6, 3, [32]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'})
	result := <-resultChan

	// Check the result
	if len(result.Data) != 9 {
		t.Errorf("Expected chunk number %d, got %d", 9, len(result.Data))
	}

	if string(result.Data[0].EncryptedData) != string(expectedChunk.EncryptedData) {
		t.Errorf("Expected data %x, got %x", expectedChunk.EncryptedData, result.Data[0].EncryptedData)
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
	resultChan, _ := ChunkBytes(fileBytes, 6, 3, [32]byte{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'a', 'b', 'c', 'd', 'e', 'f'})

	results := []types.Chunk{}

	for result := range resultChan {
		results = append(results, result)
	}

	t.Log("Chunk number: ", len(results))

	size := 0

	for i, result := range results {
		t.Log("Chunk number: ", i)
		t.Log("ECC len: ", len(result.Data))
		size += len(result.Data)
	}

	t.Log("Total size: source file: ", fileSize, " total amount of ECC chunks: ", size)
}
