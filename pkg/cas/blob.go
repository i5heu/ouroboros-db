package cas

import (
	"context"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Blob struct {
	Key     string    // Key like a path or name, this is non-unique
	Hash    hash.Hash // Hash is derived from all fields except from itself.
	Parent  hash.Hash // Key of the parent value
	Created int64     // Unix timestamp when the data was created
	chunks  []hash.Hash
}

func StoreBlob(
	ctx context.Context,
	b []byte,
	key string,
	parent hash.Hash,
	created int64,
	dr dataRouter,
	c crypt.Crypt,
	ki keyIndex,
) (Blob, error) {
	storedChunks, err := StoreChunkFromBlob(ctx, b, c, dr, ki)
	if err != nil {
		return Blob{}, err
	}

	chunkHashes := make([]hash.Hash, len(storedChunks))
	for i, chunk := range storedChunks {
		chunkHashes[i] = chunk.Hash
	}

	return Blob{
		Key:     key,
		Parent:  parent,
		Created: created,
		chunks:  chunkHashes,
	}, nil
}
