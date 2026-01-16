// Package wal defines interfaces for Write-Ahead Log operations in OuroborosDB.
package wal

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// DistributedWAL is the intake buffer for the storage system.
// It aggregates items until Block size (e.g., 16MB) is reached,
// then seals them into a Block.
type DistributedWAL interface {
	// AppendChunk adds a sealed chunk to the buffer.
	AppendChunk(ctx context.Context, chunk model.SealedChunk) error

	// AppendVertex adds a vertex to the buffer.
	AppendVertex(ctx context.Context, vertex model.Vertex) error

	// SealBlock creates a new block from the buffered items.
	// This should be called when the buffer reaches the target size.
	SealBlock(ctx context.Context) (model.Block, error)

	// GetBufferSize returns the current size of buffered data.
	GetBufferSize() int64

	// Flush forces the creation of a block even if the buffer isn't full.
	Flush(ctx context.Context) (model.Block, error)
}

// DeletionWAL logs deletions for eventual garbage collection.
type DeletionWAL interface {
	// LogDeletion records a hash for deletion.
	LogDeletion(ctx context.Context, h hash.Hash) error

	// ProcessDeletions processes pending deletions.
	ProcessDeletions(ctx context.Context) error

	// GetPendingDeletions returns the list of hashes pending deletion.
	GetPendingDeletions(ctx context.Context) ([]hash.Hash, error)
}
