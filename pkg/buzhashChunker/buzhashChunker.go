package buzhashChunker

import (
	"bytes"
	"crypto/sha512"
	"fmt"
	"io"
	"runtime"
	"sync"

	"github.com/i5heu/ouroboros-db/pkg/types"
	chunker "github.com/ipfs/boxo/chunker"
)

func ChunkBytes(data []byte) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	reader := bytes.NewReader(data)
	return ChunkReader(reader)
}

func ChunkReader(reader io.Reader) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	bz := chunker.NewBuzhash(reader)

	// get thread count
	numberOfCPUs := runtime.NumCPU()
	numberOfWorkers := (numberOfCPUs * 5) - 1

	hashChan := make(chan chunkInformation, numberOfWorkers+1)
	workerLimit := make(chan struct{}, numberOfWorkers)
	var wg sync.WaitGroup
	var collectorWg sync.WaitGroup

	// spawn collector
	resultChan := make(chan chunkResult, 1)
	collectorWg.Add(1)
	go collectChunkData(&collectorWg, hashChan, resultChan)

	// Read and process chunks
	for chunkIndex := 0; ; chunkIndex++ {
		chunk, err := bz.NextBytes()
		if err == io.EOF {
			wg.Wait()
			close(hashChan)
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error reading chunk: %w", err)
		}

		wg.Add(1)
		workerLimit <- struct{}{}
		go calculateSha512(&wg, hashChan, chunk, chunkIndex, workerLimit)
	}

	collectorWg.Wait()
	close(resultChan)

	finalResult, ok := <-resultChan
	if !ok {
		return nil, nil, fmt.Errorf("failed to read from result channel")
	}

	return finalResult.chunks, finalResult.meta, nil
}

func ChunkBytesSynchronously(data []byte) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	reader := bytes.NewReader(data)
	return ChunkReaderSynchronously(reader)
}

func ChunkReaderSynchronously(reader io.Reader) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	bz := chunker.NewBuzhash(reader)

	var chunks types.ChunkCollection
	var meta types.ChunkMetaCollection

	for chunkIndex := 0; ; chunkIndex++ {
		chunk, err := bz.NextBytes()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("error reading chunk: %w", err)
		}

		hash := sha512.Sum512(chunk)
		chunkData := types.Chunk{
			ChunkMeta: types.ChunkMeta{
				Hash:       hash,
				DataLength: uint32(len(chunk)),
			},
			Data: chunk,
		}
		chunks = append(chunks, chunkData)
		meta = append(meta, chunkData.ChunkMeta)
	}

	return chunks, meta, nil
}

type chunkInformation struct {
	chunkNumber int
	hash        types.Hash
	data        []byte
}

type chunkResult struct {
	chunks types.ChunkCollection
	meta   types.ChunkMetaCollection
}

func collectChunkData(collectorWg *sync.WaitGroup, chunkChan <-chan chunkInformation, resultChan chan<- chunkResult) {
	defer collectorWg.Done()

	chunkMap := map[int]types.Chunk{}

	for hashInfo := range chunkChan {
		chunkMap[hashInfo.chunkNumber] = types.Chunk{
			ChunkMeta: types.ChunkMeta{
				Hash:       hashInfo.hash,
				DataLength: uint32(len(hashInfo.data)),
			},
			Data: hashInfo.data,
		}
	}

	chunks := make(types.ChunkCollection, len(chunkMap))
	meta := make(types.ChunkMetaCollection, len(chunkMap))
	for i := 0; i < len(chunkMap); i++ {
		chunks[i] = chunkMap[i]
		meta[i] = chunkMap[i].ChunkMeta
	}

	resultChan <- chunkResult{
		chunks: chunks,
		meta:   meta,
	}
}

func calculateSha512(wg *sync.WaitGroup, hashChan chan<- chunkInformation, data []byte, chunkNumber int, workerLimit chan struct{}) {
	defer wg.Done()

	hash := sha512.Sum512(data)
	hashChan <- chunkInformation{
		chunkNumber: chunkNumber,
		hash:        hash,
		data:        data,
	}
	<-workerLimit
}
