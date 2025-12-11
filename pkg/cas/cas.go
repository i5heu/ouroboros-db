package cas

import (
	"context"
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type dataRouter interface {
	GetBlob(hash hash.Hash) (Blob, error)
	SetBlob(ctx context.Context, blob Blob) error

	GetChunk(hash hash.Hash) (Chunk, error)
	SetChunk(ctx context.Context, chunk Chunk) error

	GetSealedSlicesForChunk(chunkHash hash.Hash) ([]SealedSlice, error)
	GetSealedSlice(hash hash.Hash) (SealedSlice, error)
	SetSealedSlice(ctx context.Context, slice SealedSlice) error
}

type CAS struct {
	dr dataRouter
	ki keyIndex
}

func NewCAS(dr dataRouter, ki keyIndex) *CAS {
	return &CAS{
		dr: dr,
		ki: ki,
	}
}

func (cas *CAS) StoreBlob(
	ctx context.Context,
	b []byte,
	key string,
	parent hash.Hash,
	created int64, // Unix timestamp in ms
	c crypt.Crypt,
) (Blob, error) {
	if b == nil {
		return Blob{}, fmt.Errorf("cannot store nil blob")
	}
	// check if created is valid
	if created <= 0 {
		return Blob{}, fmt.Errorf("invalid created timestamp: %d", created)
	}

	storedChunks, err := cas.storeChunksFromBlob(ctx, b, c)
	if err != nil {
		return Blob{}, err
	}

	chunkHashes := make([]hash.Hash, len(storedChunks))
	for i, chunk := range storedChunks {
		chunkHashes[i] = chunk.Hash
	}

	blob := NewBlob(
		cas.dr,
		cas.ki,
		key,
		parent,
		created,
		chunkHashes,
	)

	return blob, nil
}

func (cas *CAS) GetBlob(ctx context.Context, hash hash.Hash) (Blob, error) {
	blob, err := cas.dr.GetBlob(hash)
	if err != nil {
		return Blob{}, err
	}
	return blob, nil
}
