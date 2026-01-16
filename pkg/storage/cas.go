// Package storage defines interfaces for content storage in OuroborosDB.
//
// This package provides the core storage abstractions that form the backbone
// of the OuroborosDB storage system. It defines two primary interfaces:
//
//   - CAS: High-level content operations (store, retrieve, delete)
//   - BlockStore: Low-level block and slice persistence
//
// # Architecture Overview
//
// The storage layer is designed with a clear separation of concerns:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                         CAS                             │
//	│   (High-level API: content, vertices, access control)   │
//	├─────────────────────────────────────────────────────────┤
//	│  DistributedWAL  │  EncryptionService  │  DataRouter    │
//	├─────────────────────────────────────────────────────────┤
//	│                      BlockStore                         │
//	│        (Low-level: blocks, slices, regions)             │
//	└─────────────────────────────────────────────────────────┘
//
// # CAS as Central Access Point
//
// The CAS interface is the primary entry point for all vertex data operations.
// Applications should use CAS methods rather than directly accessing the
// underlying BlockStore, as CAS handles:
//
//   - Content chunking and reassembly
//   - Encryption/decryption coordination
//   - WAL buffering for write durability
//   - Index updates for parent-child relationships
//   - Access control via KeyEntry management
package storage

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// CAS (Content-Addressable Storage) is the main management layer for content operations.
//
// CAS provides the high-level API for storing, retrieving, and managing content
// in OuroborosDB. It serves as the central access point for all vertex data
// operations, coordinating between multiple subsystems:
//
//   - EncryptionService: Handles Chunk ↔ SealedChunk transformation
//   - DistributedWAL: Buffers writes until block size is reached
//   - BlockStore: Persists blocks and provides region-based retrieval
//   - Index: Maintains parent-child relationships and key mappings
//
// # Store Operation Flow
//
//  1. StoreContent receives raw bytes and parent reference
//  2. Content is split into Chunks using content-defined chunking
//  3. Each Chunk is encrypted into a SealedChunk via EncryptionService
//  4. KeyEntry records are created for authorized users
//  5. SealedChunks and the Vertex are buffered in the WAL
//  6. When the WAL buffer is full, a Block is sealed and persisted
//  7. The Vertex hash is returned to the caller
//
// # Retrieve Operation Flow
//
//  1. GetContent receives a vertex hash
//  2. The Vertex is retrieved (via GetVertex)
//  3. For each ChunkHash in the Vertex:
//     a. Locate the SealedChunk in the BlockStore
//     b. Retrieve the KeyEntry for the requesting user
//     c. Decrypt the SealedChunk to get the Chunk
//  4. Chunks are concatenated in order
//  5. The cleartext content is returned
//
// # Thread Safety
//
// Implementations must be safe for concurrent use from multiple goroutines.
// The WAL and BlockStore implementations handle their own synchronization.
//
// # Error Handling
//
// All methods return errors for:
//   - Invalid input (nil content, zero hash)
//   - Access denied (no KeyEntry for user)
//   - Storage failures (disk full, corruption)
//   - Network failures (in distributed mode)
type CAS interface {
	// StoreContent stores content and returns the created Vertex.
	//
	// The content is processed through the full storage pipeline:
	// chunking → encryption → WAL buffering → block storage.
	//
	// Parameters:
	//   - content: The raw bytes to store (must not be nil)
	//   - parentHash: The parent vertex hash (use zero hash for root vertices)
	//
	// Returns:
	//   - The created Vertex with its computed hash
	//   - Error if storage fails
	//
	// The returned Vertex can be used to retrieve the content later
	// via GetContent(vertex.Hash).
	StoreContent(ctx context.Context, content []byte, parentHash hash.Hash) (model.Vertex, error)

	// GetContent retrieves the cleartext content for a vertex.
	//
	// This method handles the full retrieval pipeline:
	// locate blocks → retrieve SealedChunks → decrypt → reassemble.
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to retrieve
	//
	// Returns:
	//   - The original cleartext content
	//   - Error if the vertex doesn't exist or decryption fails
	//
	// Note: The caller must have a KeyEntry granting access to the chunks.
	// Access denied errors are returned if no valid KeyEntry exists.
	GetContent(ctx context.Context, vertexHash hash.Hash) ([]byte, error)

	// DeleteContent marks content for deletion via the DeletionWAL.
	//
	// Deletion is not immediate; the content is marked for garbage collection
	// and will be removed during the next deletion processing cycle.
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to delete
	//
	// Returns:
	//   - Error if the deletion cannot be logged
	//
	// Note: Deleting a vertex does not automatically delete its children.
	// The parent-child relationship is severed, but child vertices remain
	// accessible if their hashes are known.
	DeleteContent(ctx context.Context, vertexHash hash.Hash) error

	// GetVertex retrieves a vertex by its hash.
	//
	// This returns the Vertex metadata without decrypting the content.
	// Use this for DAG traversal operations that don't need content.
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to retrieve
	//
	// Returns:
	//   - The Vertex with its metadata (Parent, Created, ChunkHashes)
	//   - Error if the vertex doesn't exist
	GetVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error)

	// ListChildren returns all vertices that have the given hash as their parent.
	//
	// This enables DAG traversal by returning all direct children of a vertex.
	// The children are returned in no guaranteed order.
	//
	// Parameters:
	//   - parentHash: The hash of the parent vertex
	//
	// Returns:
	//   - Slice of child Vertices (may be empty if no children)
	//   - Error if the lookup fails
	ListChildren(ctx context.Context, parentHash hash.Hash) ([]model.Vertex, error)
}
