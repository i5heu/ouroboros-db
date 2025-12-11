package cas

import (
	"bytes"
	"context"
	"fmt"
	"io"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	buzChunker "github.com/ipfs/boxo/chunker"
)

type Chunk struct {
	ki      keyIndex
	dr      dataRouter
	Hash    hash.Hash // Hash of the content
	content []byte    // cached content
}

func NewChunk(dr dataRouter, ki keyIndex, h hash.Hash) Chunk {
	return Chunk{
		dr:   dr,
		ki:   ki,
		Hash: h,
	}
}

func (c *Chunk) GetContent(cr []crypt.Crypt) ([]byte, error) {
	if c.content != nil {
		return c.content, nil
	}

	// sealedSlices, err := c.dr.GetSealedSlicesForChunk(c.Hash)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to get sealed slices for chunk: %w", err)
	// }

	// for _, sealedSlice := range sealedSlices {
	// 	// TODO implement logic to select the correct slices for reconstruction
	// }

	// c.content = content
	return c.content, nil
}

func (cas *CAS) storeChunksFromBlob(
	ctx context.Context,
	blobByte []byte,
	c crypt.Crypt,
) ([]Chunk, error) {
	// Check if dr or ki are nil
	if cas.dr == nil {
		return nil, fmt.Errorf("dataRouter is nil")
	}
	if cas.ki == nil {
		return nil, fmt.Errorf("keyIndex is nil")
	}

	// Chunk
	chunkBytes, err := chunker(blobByte)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk blob: %w", err)
	}

	type HashAndSlices struct {
		ChunkHash hash.Hash

		// TODO: We do not really need to keep this in memory
		// but maybe we need it later for something i cannot see now
		SealedSlices []SealedSlice
	}
	hashAndSlices := []HashAndSlices{}

	for _, chunkByte := range chunkBytes {

		chunkHash := hash.HashBytes(chunkByte)

		chunkSealedSlices, err := storeSealedSlicesFromChunk(
			ctx,
			storeSealedSlicesFromChunkOpts{
				CAS:             cas,
				Crypt:           c,
				ClearChunkBytes: chunkByte,
				ChunkHash:       chunkHash,
				RSDataSlices:    4, // TODO make configurable
				RSParitySlices:  3,
			},
		)
		if err != nil {
			return nil, err
		}

		hashAndSlices = append(hashAndSlices, HashAndSlices{
			ChunkHash:    chunkHash,
			SealedSlices: chunkSealedSlices,
		})
	}

	chunks := []Chunk{}
	for _, has := range hashAndSlices {
		chunk := Chunk{
			ki:   cas.ki,
			dr:   cas.dr,
			Hash: has.ChunkHash,
		}

		err = cas.dr.SetChunk(ctx, chunk)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func chunker(payload []byte) ([][]byte, error) {
	reader := bytes.NewReader(payload)
	bz := buzChunker.NewBuzhash(reader)

	var chunks [][]byte

	for chunkIndex := 0; ; chunkIndex++ {
		buzChunk, err := bz.NextBytes()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, buzChunk)

	}
	return chunks, nil
}
