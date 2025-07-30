package storage

import (
	"bytes"
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/chunker"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/ulikunitz/xz/lzma"
)

func (s *Storage) StoreDataPipeline(data []byte, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	chunkResults, _ := chunker.ChunkBytes(data, reedSolomonDataChunks, reedSolomonParityChunks, s.dataEncryptionKey)

	var chunkData []types.Chunk

	for chunk := range chunkResults {
		chunkData = append(chunkData, chunk)
	}
	if len(chunkData) == 0 {
		return nil, nil, fmt.Errorf("Error chunking data: No chunks created")
	}

	var chunks types.ChunkCollection
	chunkMetaCollection := make(types.ChunkMetaCollection, len(chunkData))

	for i, chunk := range chunkData {
		chunkMeta := types.ChunkMeta{
			Hash: chunk.Hash,
			DataLength: 0,
		}
		chunkMetaCollection[i] = chunkMeta

		chunks = append(chunks, types.Chunk{
			ChunkMeta: chunkMeta,
			Data:      chunk.Data,
		})
	}

	return chunks, chunkMetaCollection, nil
}

func compressWithLzma(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := lzma.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		return nil, err
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func decompressWithLzma(data []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
