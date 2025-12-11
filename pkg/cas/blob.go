package cas

import (
	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Blob struct {
	ki      keyIndex
	dr      dataRouter
	Key     string      // Key like a path or name, this is non-unique
	Hash    hash.Hash   // Hash is derived from all fields except from itself.
	Parent  hash.Hash   // Key of the parent value
	Created int64       // Unix timestamp when the data was created
	chunks  []hash.Hash // cache of chunk hashes
}

func NewBlob(
	dr dataRouter,
	ki keyIndex,
	key string,
	parent hash.Hash,
	created int64,
	chunks []hash.Hash, // optional
) Blob {
	return Blob{
		dr:      dr,
		ki:      ki,
		Key:     key,
		Parent:  parent,
		Created: created,
		chunks:  chunks,
	}
}

func (b *Blob) GetChunkHashes() ([]hash.Hash, error) {
	if b.chunks != nil {
		return b.chunks, nil
	}

	blob, err := b.dr.GetBlob(b.Hash)
	if err != nil {
		return nil, err
	}
	b.chunks = blob.chunks
	return b.chunks, nil
}

func (b *Blob) GetContent(c crypt.Crypt) ([]byte, error) {
	var content []byte
	chunkHashes, err := b.GetChunkHashes()
	if err != nil {
		return nil, err
	}

	for _, chunkHash := range chunkHashes {
		chunkData, err := b.dr.GetChunk(chunkHash)
		if err != nil {
			return nil, err
		}
		chunkContent, err := chunkData.GetContent(c)
		if err != nil {
			return nil, err
		}
		content = append(content, chunkContent...)
	}

	return content, nil
}
