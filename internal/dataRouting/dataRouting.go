// Package routing provides data routing implementations for OuroborosDB.
package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/datarouter"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// DefaultDataRouter implements the DataRouter interface.
type DefaultDataRouter struct {
	mu       sync.RWMutex
	vertices map[hash.Hash]model.Vertex
	blocks   map[hash.Hash]model.Block
}

// NewDataRouter creates a new DefaultDataRouter instance.
func NewDataRouter() *DefaultDataRouter {
	return &DefaultDataRouter{
		vertices: make(map[hash.Hash]model.Vertex),
		blocks:   make(map[hash.Hash]model.Block),
	}
}

// StoreVertex stores a vertex and returns its hash.
func (r *DefaultDataRouter) StoreVertex(ctx context.Context, vertex model.Vertex) (hash.Hash, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if vertex.Hash == (hash.Hash{}) {
		return hash.Hash{}, fmt.Errorf("datarouter: vertex hash is required")
	}

	r.vertices[vertex.Hash] = vertex
	return vertex.Hash, nil
}

// RetrieveVertex retrieves a vertex by its hash.
func (r *DefaultDataRouter) RetrieveVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vertex, exists := r.vertices[vertexHash]
	if !exists {
		return model.Vertex{}, fmt.Errorf("datarouter: vertex %s not found", vertexHash)
	}
	return vertex, nil
}

// DeleteVertex marks a vertex for deletion.
func (r *DefaultDataRouter) DeleteVertex(ctx context.Context, vertexHash hash.Hash) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.vertices, vertexHash)
	return nil
}

// DistributeBlockSlices distributes block slices to appropriate nodes.
func (r *DefaultDataRouter) DistributeBlockSlices(ctx context.Context, block model.Block) error {
	// Implementation will distribute slices across the cluster
	// This is a placeholder for the actual implementation
	r.mu.Lock()
	defer r.mu.Unlock()

	r.blocks[block.Hash] = block
	return nil
}

// RetrieveBlock retrieves a block, potentially reconstructing from slices.
func (r *DefaultDataRouter) RetrieveBlock(ctx context.Context, blockHash hash.Hash) (model.Block, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	block, exists := r.blocks[blockHash]
	if !exists {
		return model.Block{}, fmt.Errorf("datarouter: block %s not found", blockHash)
	}
	return block, nil
}

// Ensure DefaultDataRouter implements the DataRouter interface.
var _ datarouter.DataRouter = (*DefaultDataRouter)(nil)
