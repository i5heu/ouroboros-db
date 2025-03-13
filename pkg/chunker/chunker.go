package chunker

import (
	"bytes"
	"fmt"
	"io"

	chunker "github.com/ipfs/boxo/chunker"
	"github.com/klauspost/reedsolomon"
)

type ChunkInformation struct {
	ChunkNumber             uint32
	ReedSolomonDataChunks   uint8
	ReedSolomonParityChunks uint8
	ReedSolomonParityShard  uint64
	Data                    []byte
}

func ChunkBytes(data []byte, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (chan ChunkInformation, chan error) {
	reader := bytes.NewReader(data)
	return ChunkReader(reader, reedSolomonDataChunks, reedSolomonParityChunks)
}

// ChunkReader will chunk the data from the reader and return the chunks through a channel
// The chunk size is minimum/maximum = 128K/512K
func ChunkReader(reader io.Reader, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (chan ChunkInformation, chan error) {
	resultChan := make(chan ChunkInformation, 20)
	errorChan := make(chan error)
	bz := chunker.NewBuzhash(reader)

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		for chunkIndex := 0; ; chunkIndex++ {
			chunk, err := bz.NextBytes()
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

			shards, err := enc.Split(chunk)
			if err != nil {
				errorChan <- fmt.Errorf("error splitting chunk: %w", err)
				return
			}

			err = enc.Encode(shards)
			if err != nil {
				errorChan <- fmt.Errorf("error encoding chunk: %w", err)
				return
			}

			for i := range shards {
				chunkData := ChunkInformation{
					ChunkNumber:             uint32(chunkIndex),
					ReedSolomonDataChunks:   reedSolomonDataChunks,
					ReedSolomonParityChunks: reedSolomonParityChunks,
					ReedSolomonParityShard:  uint64(i),
					Data:                    shards[i],
				}

				resultChan <- chunkData
			}

		}
	}()

	return resultChan, errorChan
}
