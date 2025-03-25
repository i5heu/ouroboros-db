package chunker

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-db/pkg/types"
	chunker "github.com/ipfs/boxo/chunker"
	"github.com/klauspost/reedsolomon"
)

type ECChunk struct {
	ECChunkHash    types.Hash
	OtherECCHashes []types.Hash

	BuzChunkNumber uint32
	BuzChunkHash   types.Hash

	ECCNumber uint8
	ECC_k     uint8
	ECC_n     uint8
	Data      []byte
}

func ChunkBytes(data []byte, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (chan ECChunk, chan error) {
	reader := bytes.NewReader(data)
	return ChunkReader(reader, reedSolomonDataChunks, reedSolomonParityChunks)
}

// ChunkReader will chunk the data from the reader and return the chunks through a channel
// The chunk size is minimum/maximum = 128K/512K
func ChunkReader(reader io.Reader, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (chan ECChunk, chan error) {
	resultChan := make(chan ECChunk, 20)
	errorChan := make(chan error)
	bz := chunker.NewBuzhash(reader)

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		for chunkIndex := 0; ; chunkIndex++ {
			buzChunk, err := bz.NextBytes()
			if err == io.EOF {
				break
			}
			if err != nil {
				errorChan <- fmt.Errorf("error reading chunk: %w", err)
				return
			}

			// Create an encoder with 7 data and 3 parity slices.
			enc, err := reedsolomon.New(int(reedSolomonDataChunks), int(reedSolomonParityChunks))
			if err != nil {
				errorChan <- fmt.Errorf("error creating reed solomon encoder: %w", err)
				return
			}

			shards, err := enc.Split(buzChunk)
			if err != nil {
				errorChan <- fmt.Errorf("error splitting chunk: %w", err)
				return
			}

			err = enc.Encode(shards)
			if err != nil {
				errorChan <- fmt.Errorf("error encoding chunk: %w", err)
				return
			}

			buzChunkHash := sha512.Sum512(buzChunk)

			eccHashes := make([]types.Hash, reedSolomonDataChunks+reedSolomonParityChunks)
			for i := range eccHashes {
				eccHashes[i] = types.Hash(sha512.Sum512(shards[i]))
			}

			for i := range shards {
				chunkData := ECChunk{
					ECChunkHash:    types.Hash(sha512.Sum512(shards[i])),
					OtherECCHashes: filterOutCurrentECCHash(eccHashes, uint8(i)),

					BuzChunkHash:   buzChunkHash,
					BuzChunkNumber: uint32(chunkIndex),

					ECCNumber: reedSolomonDataChunks,
					ECC_k:     reedSolomonParityChunks,
					ECC_n:     uint8(i),
					Data:      shards[i],
				}

				resultChan <- chunkData
			}

		}
	}()

	return resultChan, errorChan
}

func filterOutCurrentECCHash(eccHashes []types.Hash, currentECCNumber uint8) []types.Hash {
	filteredHashes := make([]types.Hash, 0, len(eccHashes)-1)
	for i := range eccHashes {
		if uint8(i) != currentECCNumber {
			filteredHashes = append(filteredHashes, eccHashes[i])
		}
	}
	return filteredHashes
}
