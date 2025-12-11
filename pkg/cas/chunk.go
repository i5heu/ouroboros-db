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

func (c *Chunk) GetContent(cr crypt.Crypt) ([]byte, error) {
	if c.content != nil {
		return c.content, nil
	}

	sealedSlices, err := c.dr.GetSealedSlicesForChunk(c.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get sealed slices for chunk: %w", err)
	}

	var content []byte
	for _, sealedSlice := range sealedSlices {
		sliceContent, err := sealedSlice.Decrypt(cr)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt sealed slice: %w", err)
		}
		content = append(content, sliceContent...)
	}

	c.content = content
	return c.content, nil
}

func StoreChunkFromBlob(
	ctx context.Context,
	blobByte []byte,
	c crypt.Crypt,
	dr dataRouter,
	ki keyIndex,
) ([]Chunk, error) {
	// Check if dr or ki are nil
	if dr == nil {
		return nil, fmt.Errorf("dataRouter is nil")
	}
	if ki == nil {
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

	for i, chunkByte := range chunkBytes {

		chunkHash := hash.HashBytes(chunkByte)

		chunkSealedSlices, err := StoreSealedSlicesFromChunk(
			ctx,
			StoreSealedSlicesFromChunkOpts{
				CAS:             &CAS{dr: dr, ki: ki},
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

		hashAndSlices[i] = HashAndSlices{
			ChunkHash:    chunkHash,
			SealedSlices: chunkSealedSlices,
		}
	}

	chunks := []Chunk{}
	for _, has := range hashAndSlices {
		chunk := Chunk{
			dr:   dr,
			Hash: has.ChunkHash,
		}

		err = dr.SetChunk(chunk)
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
