// Package model defines the core data types used throughout OuroborosDB.
// These types represent the logical and physical data structures in the system.
package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// Vertex represents a logical node in the DAG (Directed Acyclic Graph).
// It is stored UNENCRYPTED in the VertexSection of a Block.
// Vertex replaces the old "Blob" concept in the new architecture.
type Vertex struct {
	// Hash is the unique identifier for this vertex, derived from its contents.
	Hash hash.Hash

	// Parent is the hash of the parent vertex in the DAG.
	Parent hash.Hash

	// Created is the Unix timestamp (in milliseconds) when this vertex was created.
	Created int64

	// ChunkHashes contains the hashes of all chunks that make up this vertex's content.
	ChunkHashes []hash.Hash
}

// GetChunkHashes returns the chunk hashes associated with this vertex.
func (v *Vertex) GetChunkHashes() []hash.Hash {
	return v.ChunkHashes
}
