// Package model defines the core data types used throughout OuroborosDB.
//
// This package contains the logical and physical data structures that form the
// foundation of the storage system. These types are used throughout the CAS
// (Content-Addressable Storage) layer and are central to the vertex-based
// data model.
//
// # Data Model Overview
//
// The data model is organized around these key concepts:
//
//   - Vertex: The logical unit in the DAG, containing metadata and references to content
//   - Chunk: Cleartext content (memory-only, never persisted directly)
//   - SealedChunk: Encrypted content with integrity verification
//   - Block: The archive structure containing multiple SealedChunks and Vertices
//   - BlockSlice: Reed-Solomon shards of Blocks for distribution
//   - KeyEntry: Access control mapping users to content
//
// # Vertex Access via CAS
//
// Vertices are accessed through the CAS (Content-Addressable Storage) interface,
// which serves as the central access point for all vertex data operations.
// The CAS coordinates encryption, WAL buffering, block storage, and access control.
package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// Vertex represents a logical node in the DAG (Directed Acyclic Graph).
//
// A Vertex is the fundamental unit of the logical data hierarchy. It stores
// metadata about content (parent relationships, timestamps) and references
// to the actual content via ChunkHashes. Vertices form a DAG structure where
// each vertex can have one parent (except root vertices) and multiple children.
//
// # Storage Characteristics
//
// Vertices are stored UNENCRYPTED in the VertexSection of a Block. This allows
// the system to traverse the DAG structure and resolve parent-child relationships
// without requiring decryption keys. Only the actual content (stored in Chunks)
// is encrypted.
//
// # Hash Derivation
//
// The Hash field is derived from:
//   - Parent hash
//   - Created timestamp
//   - ChunkHashes (ordered)
//
// This ensures content-addressability: identical content with the same metadata
// will always produce the same hash.
//
// # Usage
//
// Vertices should be retrieved and stored via the CAS interface:
//
//	vertex, err := cas.GetVertex(ctx, vertexHash)
//	children, err := cas.ListChildren(ctx, parentHash)
type Vertex struct {
	// Hash is the unique identifier for this vertex, derived from its contents.
	// It is computed from Parent, Created, and ChunkHashes to ensure
	// content-addressability.
	Hash hash.Hash

	// Parent is the hash of the parent vertex in the DAG.
	// For root vertices, this is the zero hash.
	Parent hash.Hash

	// Created is the Unix timestamp (in milliseconds) when this vertex was created.
	// This timestamp is part of the hash computation to ensure uniqueness
	// even for identical content.
	Created int64

	// ChunkHashes contains the ordered list of hashes for all chunks that
	// make up this vertex's content. The content can be reconstructed by
	// retrieving and concatenating these chunks in order.
	ChunkHashes []hash.Hash
}

// GetChunkHashes returns the chunk hashes associated with this vertex.
// The returned slice should be used to retrieve the actual content
// via the CAS or BlockStore.
func (v *Vertex) GetChunkHashes() []hash.Hash {
	return v.ChunkHashes
}
