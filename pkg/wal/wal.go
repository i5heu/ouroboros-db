// Package wal defines interfaces for Write-Ahead Log operations in OuroborosDB.
//
// The WAL (Write-Ahead Log) system provides durability and batching for storage
// operations. It consists of two complementary interfaces:
//
//   - DistributedWAL: Buffers writes and creates Blocks
//   - DeletionWAL: Logs deletions for garbage collection
//
// # DistributedWAL Purpose
//
// The DistributedWAL serves as an intake buffer that aggregates small writes
// into efficient large Blocks. This provides several benefits:
//
//   - Write amplification reduction: Many small writes become one large Block
//   - Durability: Items are persisted to the WAL before acknowledgment
//   - Batching efficiency: Block-level operations are more efficient
//   - Crash recovery: Uncommitted items can be recovered from the WAL
//
// # Block Creation Flow
//
//  1. CAS encrypts content into SealedChunks
//  2. SealedChunks and Vertices are appended to the WAL
//  3. When buffer reaches target size (e.g., 16MB), SealBlock() is called
//  4. SealBlock() creates a Block with all buffered items
//  5. The Block is passed to BlockStore for persistence
//  6. WAL buffer is cleared for new items
//
// # DeletionWAL Purpose
//
// The DeletionWAL provides safe deletion with garbage collection semantics:
//
//   - Deletions are logged, not executed immediately
//   - Periodic garbage collection processes pending deletions
//   - Allows for deletion recovery before GC runs
//   - Prevents inconsistent state during crashes
package wal

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// DistributedWAL is the intake buffer for the storage system.
//
// DistributedWAL aggregates SealedChunks and Vertices until the buffer reaches
// the target block size (typically 16MB), then seals them into a Block. This
// batching dramatically improves storage efficiency and provides write
// durability.
//
// # Write Flow
//
//	StoreContent → Encrypt → AppendChunk + AppendVertex → [buffer fills]
//
// → SealBlock
//
// # Buffer Management
//
// The buffer accumulates items until:
//   - GetBufferSize() >= target block size → automatic SealBlock()
//   - Explicit Flush() call → force SealBlock() regardless of size
//
// # Distribution
//
// "Distributed" refers to the WAL's role in the distributed system:
//   - The WAL may replicate entries to peer nodes for durability
//   - SealBlock() may coordinate with the cluster for consistency
//   - Recovery may involve fetching entries from peer WALs
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Multiple goroutines may
// append items simultaneously.
//
// # Durability Guarantees
//
// Items appended to the WAL are durable (persisted) before the append returns.
// This ensures that acknowledged writes survive crashes. The exact durability
// depends on the implementation (local disk, replicated, etc.).
type DistributedWAL interface {
	// AppendChunk adds a sealed chunk to the buffer.
	//
	// The chunk is added to the WAL buffer and persisted for durability.
	// When the buffer reaches the target size, SealBlock() should be called.
	//
	// Parameters:
	//   - chunk: The encrypted chunk to buffer
	//
	// Returns:
	//   - Error if the append fails (WAL full, I/O error)
	AppendChunk(ctx context.Context, chunk model.SealedChunk) error

	// AppendVertex adds a vertex to the buffer.
	//
	// The vertex is added to the WAL buffer along with its associated chunks.
	// Vertices reference their chunks via ChunkHashes.
	//
	// Parameters:
	//   - vertex: The vertex metadata to buffer
	//
	// Returns:
	//   - Error if the append fails
	AppendVertex(ctx context.Context, vertex model.Vertex) error

	// AppendKeyEntry adds a key entry to the buffer.
	//
	// The key entry is added to the WAL buffer. Key entries provide access
	// control for encrypted chunks.
	//
	// Parameters:
	//   - keyEntry: The key entry to buffer
	//
	// Returns:
	//   - Error if the append fails
	AppendKeyEntry(ctx context.Context, keyEntry model.KeyEntry) error

	// SealBlock creates a new Block from all buffered items.
	//
	// This is called when the buffer reaches the target size. It:
	//  1. Serializes all buffered SealedChunks into the DataSection
	//  2. Serializes all buffered Vertices into the VertexSection
	//  3. Builds the ChunkIndex and VertexIndex
	//  4. Computes the block hash
	//  5. Returns the complete Block AND the WAL keys
	//
	// IMPORTANT: This method does NOT clear the WAL buffer. The caller
	// must call ClearBlock() after confirming the block has been
	// successfully distributed to the required number of nodes.
	//
	// The returned Block should be passed to BlockStore for persistence
	// and to DataRouter for distribution. The walKeys should be passed
	// to ClearBlock() once distribution is confirmed.
	//
	// Returns:
	//   - The sealed Block containing all buffered items
	//   - The WAL keys that should be deleted after confirmed distribution
	//   - Error if block creation fails
	//
	// If the buffer is empty, this returns an error.
	SealBlock(ctx context.Context) (model.Block, [][]byte, error)

	// ClearBlock removes WAL entries for a successfully distributed block.
	//
	// This should only be called after confirming the block has been
	// distributed to the required number of nodes (typically 3+).
	// The walKeys parameter should come from the SealBlock() return value.
	//
	// After calling ClearBlock, the buffer size is recalculated.
	//
	// Parameters:
	//   - walKeys: The keys returned by SealBlock() for this block
	//
	// Returns:
	//   - Error if the WAL entries cannot be deleted
	ClearBlock(ctx context.Context, walKeys [][]byte) error

	// GetBufferSize returns the current size of buffered data in bytes.
	//
	// This is used to determine when to call SealBlock(). Typical usage:
	//
	//	if wal.GetBufferSize() >= targetBlockSize {
	//	    block, err := wal.SealBlock(ctx)
	//	    // ... persist and distribute block
	//	}
	//
	// Returns the approximate size of DataSection + VertexSection that
	// would be created by SealBlock().
	GetBufferSize() int64

	// Flush forces the creation of a block even if the buffer isn't full.
	//
	// This is useful for:
	//   - Ensuring data durability before shutdown
	//   - Creating a block after a period of inactivity
	//   - Testing and debugging
	//
	// Flush() is equivalent to SealBlock() but may be called regardless
	// of buffer size. If the buffer is empty, behavior is implementation-defined
	// (may return error or empty block).
	//
	// IMPORTANT: Like SealBlock(), this does NOT clear the WAL buffer.
	// The caller must call ClearBlock() after confirmed distribution.
	//
	// Returns:
	//   - The sealed Block (may be smaller than target size)
	//   - The WAL keys that should be deleted after confirmed distribution
	//   - Error if block creation fails
	Flush(ctx context.Context) (model.Block, [][]byte, error)

	// GetChunk retrieves a sealed chunk from the WAL buffer if it exists.
	//
	// This allows reading data that has been written but not yet sealed into a block.
	//
	// Parameters:
	//   - chunkHash: The hash of the chunk to retrieve
	//
	// Returns:
	//   - The sealed chunk
	//   - Error if the chunk is not found or retrieval fails
	GetChunk(ctx context.Context, chunkHash hash.Hash) (model.SealedChunk, error)

	// GetVertex retrieves a vertex from the WAL buffer if it exists.
	//
	// This allows reading vertices that have been written but not yet sealed into a block.
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to retrieve
	//
	// Returns:
	//   - The vertex
	//   - Error if the vertex is not found or retrieval fails
	GetVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error)

	// GetKeyEntry retrieves a key entry from the WAL buffer if it exists.
	//
	// Parameters:
	//   - chunkHash: The hash of the chunk the key belongs to
	//   - pubKeyHash: The hash of the public key the entry is encrypted for
	//
	// Returns:
	//   - The key entry
	//   - Error if not found
	GetKeyEntry(ctx context.Context, chunkHash, pubKeyHash hash.Hash) (model.KeyEntry, error)
}

// DeletionWAL logs deletions for eventual garbage collection.
//
// DeletionWAL provides safe, recoverable deletion semantics. Instead of
// immediately removing data, deletions are logged and processed later
// by a garbage collector. This approach provides:
//
//   - Crash safety: Incomplete deletions can be detected and completed
//   - Undo capability: Deletions can be cancelled before GC runs
//   - Consistency: Related data can be deleted atomically
//   - Distributed coordination: Deletions can be replicated before execution
//
// # Deletion Flow
//
//  1. User calls CAS.DeleteContent(vertexHash)
//  2. CAS calls DeletionWAL.LogDeletion(vertexHash)
//  3. The deletion is persisted to the WAL
//  4. Periodically, ProcessDeletions() is called
//  5. ProcessDeletions() removes the actual data
//  6. The deletion record is removed from the WAL
//
// # Distributed Considerations
//
// In a distributed deployment:
//   - Deletion logs may be replicated to ensure consistency
//   - ProcessDeletions() coordinates with peer nodes
//   - A quorum may be required before actual deletion
type DeletionWAL interface {
	// LogDeletion records a hash for deletion.
	//
	// The hash is added to the pending deletion list. The actual data
	// is not removed until ProcessDeletions() is called.
	//
	// Parameters:
	//   - h: The hash to mark for deletion (vertex, chunk, or block)
	//
	// Returns:
	//   - Error if the log entry cannot be persisted
	//
	// Logging the same hash multiple times is idempotent.
	LogDeletion(ctx context.Context, h hash.Hash) error

	// ProcessDeletions processes all pending deletions.
	//
	// This is the garbage collection operation that actually removes data.
	// It should be called periodically (e.g., by a background goroutine).
	//
	// For each pending deletion:
	//  1. Remove the data from storage
	//  2. Remove the deletion record from the WAL
	//
	// Returns:
	//   - Error if processing fails (partial progress may have been made)
	//
	// This operation may take significant time for many pending deletions.
	ProcessDeletions(ctx context.Context) error

	// GetPendingDeletions returns all hashes pending deletion.
	//
	// This is useful for:
	//   - Monitoring the deletion backlog
	//   - Implementing deletion cancellation
	//   - Debugging and testing
	//
	// Returns:
	//   - Slice of hashes that have been logged but not yet processed
	//   - Error if the list cannot be retrieved
	GetPendingDeletions(ctx context.Context) ([]hash.Hash, error)
}
