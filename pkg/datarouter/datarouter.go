// Package datarouter defines interfaces for routing data operations across the cluster.
package datarouter

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// DataRouter routes data operations across the cluster.
// It coordinates with CAS for content operations and handles
// distribution of blocks/slices to appropriate nodes.
type DataRouter interface {
	// StoreVertex stores a vertex and returns its hash.
	StoreVertex(ctx context.Context, vertex model.Vertex) (hash.Hash, error)

	// RetrieveVertex retrieves a vertex by its hash.
	RetrieveVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error)

	// DeleteVertex marks a vertex for deletion.
	DeleteVertex(ctx context.Context, vertexHash hash.Hash) error

	// DistributeBlockSlices distributes block slices to appropriate nodes.
	DistributeBlockSlices(ctx context.Context, block model.Block) error

	// RetrieveBlock retrieves a block, potentially reconstructing from slices.
	RetrieveBlock(ctx context.Context, blockHash hash.Hash) (model.Block, error)
}
