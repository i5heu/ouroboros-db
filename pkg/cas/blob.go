package cas

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Blob struct {
	Key     string    // Key like a path or name, this is non-unique
	Hash    hash.Hash // Hash is derived from all fields except from itself.
	Parent  hash.Hash // Key of the parent value
	Created int64     // Unix timestamp when the data was created
	chunks  []hash.Hash
}
