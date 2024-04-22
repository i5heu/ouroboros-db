package buzhashChunker

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"

	chunker "github.com/ipfs/boxo/chunker"
)

type ChunkData struct {
	Hash  [64]byte // SHA-512 hash
	Chunk []byte   // The actual data chunk
}

func ChunkBytes(data []byte) ([]ChunkData, error) {
	reader := bytes.NewReader(data)

	return ChunkReader(reader)
}

func ChunkReader(reader io.Reader) ([]ChunkData, error) {
	bz := chunker.NewBuzhash(reader)

	var chunks []ChunkData

	// Read and process chunks
	for {
		chunk, err := bz.NextBytes()
		if err == io.EOF {
			break // End of data reached.
		}
		if err != nil {
			return nil, fmt.Errorf("error reading chunk: %w", err)
		}

		// Compute SHA-512 hash of the chunk
		hash := sha512.Sum512(chunk)

		// Append the hash and the chunk data to the slice
		chunks = append(chunks, ChunkData{
			Hash:  hash,
			Chunk: chunk,
		})
	}

	return chunks, nil
}
