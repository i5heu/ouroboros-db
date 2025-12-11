package cas

import (
	"context"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Blob struct {
	cas     *CAS
	Key     string      // Key like a path or name, this is non-unique
	Hash    hash.Hash   // Hash is derived from all fields except from itself.
	Parent  hash.Hash   // Key of the parent value
	Created int64       // Unix timestamp when the data was created
	chunks  []hash.Hash // cache of chunk hashes
}

func NewBlob(
	cas *CAS,
	key string,
	parent hash.Hash,
	created int64,
) Blob {
	return Blob{
		cas:     cas,
		Key:     key,
		Parent:  parent,
		Created: created,
	}
}

func (b *Blob) GetChunkHashes(ctx context.Context) ([]hash.Hash, error) {
	if b.chunks != nil {
		return b.chunks, nil
	}

	blob, err := b.cas.dr.GetBlob(ctx, b.Hash)
	if err != nil {
		return nil, err
	}
	b.chunks = blob.chunks
	return b.chunks, nil
}

func (b *Blob) GetContent(ctx context.Context, c crypt.Crypt) ([]byte, error) {
	var content []byte
	chunkHashes, err := b.GetChunkHashes(ctx)
	if err != nil {
		return nil, err
	}

	for _, chunkHash := range chunkHashes {
		chunkData, err := b.cas.dr.GetChunk(ctx, chunkHash)
		if err != nil {
			return nil, err
		}
		chunkContent, err := chunkData.GetContent(ctx, c)
		if err != nil {
			return nil, err
		}
		content = append(content, chunkContent...)
	}

	return content, nil
}
