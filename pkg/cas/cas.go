package cas

import "github.com/i5heu/ouroboros-crypt/pkg/hash"

type dataRouter interface {
	GetBlob(hash hash.Hash) (Blob, error)
	SetBlob(blob Blob) error

	GetChunk(hash hash.Hash) (Chunk, error)
	SetChunk(chunk Chunk) error

	GetSealedSlicesForChunk(chunkHash hash.Hash) ([]SealedSlice, error)
	GetSealedSlice(hash hash.Hash) (SealedSlice, error)
	SetSealedSlice(slice SealedSlice) error
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
