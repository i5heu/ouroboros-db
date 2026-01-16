package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// Chunk represents cleartext content.
// Chunks only exist temporarily in memory during processing/decryption.
// They are the basic unit of content that gets encrypted into SealedChunks.
type Chunk struct {
	// Hash is the hash of the original cleartext content.
	Hash hash.Hash

	// Size is the size of the content in bytes.
	Size int

	// Content is the cleartext data (only present in memory, never persisted directly).
	Content []byte
}

// GetContent returns the cleartext content of this chunk.
func (c *Chunk) GetContent() []byte {
	return c.Content
}
