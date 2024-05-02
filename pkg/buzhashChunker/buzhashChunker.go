package buzhashChunker

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"runtime"
	"sync"

	chunker "github.com/ipfs/boxo/chunker"
)

type ChunkData struct {
	Hash [64]byte // SHA-512 hash
	Data []byte   // The actual data chunk
}

func ChunkBytes(data []byte) ([]ChunkData, error) {
	reader := bytes.NewReader(data)

	return ChunkReader(reader)
}

func ChunkReader(reader io.Reader) ([]ChunkData, error) {
	bz := chunker.NewBuzhash(reader)

	// get thread count
	numberOfCPUs := runtime.NumCPU()
	numberOfWorkers := (numberOfCPUs * 5) - 1

	hashChan := make(chan chunkInformation, numberOfWorkers+1)
	workerLimit := make(chan struct{}, numberOfWorkers)
	var wg sync.WaitGroup

	// spawn collector
	resultChan := make(chan []ChunkData, 1)
	go collectChunkData(hashChan, resultChan)

	// Read and process chunks
	for chunkIndex := 0; ; chunkIndex++ { // Use a loop-scoped index
		chunk, err := bz.NextBytes()
		if err == io.EOF {
			wg.Wait()
			close(hashChan)
			break // End of data reached.
		}
		if err != nil {
			return nil, fmt.Errorf("error reading chunk: %w", err)
		}

		wg.Add(1)
		workerLimit <- struct{}{}
		go calculateSha512(&wg, hashChan, chunk, chunkIndex, workerLimit)
	}

	return <-resultChan, nil
}

type chunkInformation struct {
	chunkNumber int
	hash        [64]byte
	data        []byte
}

func collectChunkData(chunkChan <-chan chunkInformation, resultChan chan<- []ChunkData) {

	chunkMap := map[int]ChunkData{}

	for hashInfo := range chunkChan {
		chunkMap[hashInfo.chunkNumber] = ChunkData{
			Hash: hashInfo.hash,
			Data: hashInfo.data,
		}
	}

	// Convert map to slice
	chunks := make([]ChunkData, len(chunkMap))
	for i := 0; i < len(chunkMap); i++ {
		chunks[i] = chunkMap[i]
	}

	resultChan <- chunks
	return
}

func calculateSha512(wg *sync.WaitGroup, hashChan chan<- chunkInformation, data []byte, chunkNumber int, workerLimit chan struct{}) {
	defer wg.Done()

	hash := sha512.Sum512(data)
	hashChan <- chunkInformation{
		chunkNumber: chunkNumber,
		hash:        hash,
	}
	<-workerLimit

	return
}

func (c ChunkData) PrettyPrint() string {
	return fmt.Sprintf("ChunkData{Hash: %x, Data(length): %x}", hex.EncodeToString(c.Hash[:]), len(c.Data))
}
