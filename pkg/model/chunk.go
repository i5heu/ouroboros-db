package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// Chunk represents cleartext content before encryption.
//
// Chunks are the fundamental unit of content storage. They exist only
// temporarily in memory during content processing:
//
//   - During storage: content is split into Chunks, then encrypted into SealedChunks
//   - During retrieval: SealedChunks are decrypted back into Chunks
//
// # Security Model
//
// Chunks contain cleartext data and should NEVER be persisted directly to storage.
// All persistent storage uses SealedChunks (encrypted form). Chunks exist only:
//   - In memory during the encryption pipeline (store operation)
//   - In memory during the decryption pipeline (retrieve operation)
//
// # Content Chunking
//
// Large content is split into multiple Chunks using Buzhash content-defined
// chunking. This provides:
//   - Deduplication: identical content regions produce identical chunks
//   - Efficient updates: changes affect only modified chunks
//   - Parallelization: chunks can be processed independently
//
// # Hash Computation
//
// The Hash is computed directly from the cleartext Content bytes, ensuring
// that identical content always produces the same hash regardless of when
// or where it was created.
type Chunk struct {
	// Hash is the cryptographic hash of the cleartext content.
	// This hash is used to reference the chunk from Vertices and
	// to verify content integrity after decryption.
	Hash hash.Hash

	// Size is the size of the content in bytes.
	// This is stored separately to allow size queries without
	// loading the full content into memory.
	Size int

	// Content is the cleartext data.
	// WARNING: This field contains unencrypted data and should only
	// exist in memory. Never persist Chunk objects directly.
	Content []byte
}

// GetContent returns the cleartext content of this chunk.
//
// The returned slice is a direct reference to the internal content;
// callers should not modify it.
func (c *Chunk) GetContent() []byte {
	return c.Content
}
