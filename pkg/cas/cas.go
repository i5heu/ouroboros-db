// Package cas implements the Content-Addressable Storage (CAS) layer used by
// OuroborosDB.
//
// The CAS is responsible for storing and retrieving blobs, chunks and sealed
// slices. It provides APIs to:
// - store and fetch blobs (logical objects with metadata and chunk references),
//   - chunk blob payloads and persist chunks,
//   - store and find sealed slices (sealed units of storage), and
//   - interact with a dataRouter and keyIndex to persist and look up objects.
//
// CAS ensures content-addressability via cryptographic hashes and acts as the
// library-level facade for higher-level operations; higher-level packages
// should
// use `NewCAS` and the CAS methods to interact with persisted content.
//
// Note: CAS itself does not implement on-disk layout; it delegates persistence
// to the configured dataRouter implementation.
package cas

import (
	"context"
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type dataRouter interface {
	GetBlob(ctx context.Context, hash hash.Hash) (Blob, error)
	SetBlob(ctx context.Context, blob Blob) error

	GetChunk(ctx context.Context, hash hash.Hash) (Chunk, error)
	SetChunk(ctx context.Context, chunk Chunk) error

	GetSealedSlicesForChunk(
		ctx context.Context,
		chunkHash hash.Hash,
	) ([]SealedSlice, error)

	GetSealedSlice(ctx context.Context, hash hash.Hash) (SealedSlice, error)
	SetSealedSlice(ctx context.Context, slice SealedSlice) error
	GetSealedSlicePayload(ctx context.Context, hash hash.Hash) ([]byte, error)
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
		cas,
		key,
		parent,
		created,
	)

	err = cas.dr.SetBlob(ctx, blob)
	if err != nil {
		return Blob{}, err
	}

	return blob, nil
}

func (cas *CAS) GetBlob(ctx context.Context, hash hash.Hash) (Blob, error) {
	blob, err := cas.dr.GetBlob(ctx, hash)
	if err != nil {
		return Blob{}, err
	}
	return blob, nil
}
