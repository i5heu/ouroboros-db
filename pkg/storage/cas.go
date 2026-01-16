// Package storage defines interfaces for content storage in OuroborosDB.
package storage

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// CAS (Content Addressable Storage) is the main management layer for content operations.
// It provides high-level API for content operations, coordinating encryption,
// WAL buffering, block storage, and access control.
type CAS interface {
	// StoreContent stores content and returns the created vertex.
	// The content is chunked, encrypted, and buffered in the WAL.
	StoreContent(ctx context.Context, content []byte, parentHash hash.Hash) (model.Vertex, error)

	// GetContent retrieves the cleartext content for a vertex.
	GetContent(ctx context.Context, vertexHash hash.Hash) ([]byte, error)

	// DeleteContent marks content for deletion.
	DeleteContent(ctx context.Context, vertexHash hash.Hash) error

	// GetVertex retrieves a vertex by its hash.
	GetVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error)

	// ListChildren returns all vertices that have the given hash as their parent.
	ListChildren(ctx context.Context, parentHash hash.Hash) ([]model.Vertex, error)
}
