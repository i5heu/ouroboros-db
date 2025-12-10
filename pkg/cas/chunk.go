package cas

import "github.com/i5heu/ouroboros-crypt/pkg/hash"

type Chunk struct {
	Hash         hash.Hash
	sealedSlices []hash.Hash
}

