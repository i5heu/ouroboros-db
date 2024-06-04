package buzhashChunker

import (
	"bytes"
	"fmt"
	"io"

	chunker "github.com/ipfs/boxo/chunker"
)

type ChunkInformation struct {
	ChunkNumber int
	Data        []byte
}

func ChunkBytes(data []byte) (chan ChunkInformation, chan error) {
	reader := bytes.NewReader(data)
	return ChunkReader(reader)
}

func ChunkReader(reader io.Reader) (chan ChunkInformation, chan error) {
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

			chunkData := ChunkInformation{
				ChunkNumber: chunkIndex,
				Data:        chunk,
			}

			resultChan <- chunkData
		}
	}()

	return resultChan, errorChan
}
