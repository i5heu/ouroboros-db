// Package wal provides Write-Ahead Log implementations for OuroborosDB.
package wal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/i5heu/ouroboros-db/pkg/wal"
)

// DefaultBlockSize is the target size for blocks (16MB).
const DefaultBlockSize = 16 * 1024 * 1024

// DefaultDistributedWAL implements the DistributedWAL interface.
type DefaultDistributedWAL struct {
	mu           sync.Mutex
	chunks       []model.SealedChunk
	vertices     []model.Vertex
	bufferSize   int64
	targetSize   int64
}

// NewDistributedWAL creates a new DefaultDistributedWAL instance.
func NewDistributedWAL() *DefaultDistributedWAL {
	return &DefaultDistributedWAL{
		chunks:     make([]model.SealedChunk, 0),
		vertices:   make([]model.Vertex, 0),
		targetSize: DefaultBlockSize,
	}
}

// AppendChunk adds a sealed chunk to the buffer.
func (w *DefaultDistributedWAL) AppendChunk(ctx context.Context, chunk model.SealedChunk) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.chunks = append(w.chunks, chunk)
	w.bufferSize += int64(len(chunk.EncryptedContent))
	return nil
}

// AppendVertex adds a vertex to the buffer.
func (w *DefaultDistributedWAL) AppendVertex(ctx context.Context, vertex model.Vertex) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.vertices = append(w.vertices, vertex)
	// Estimate vertex size (simplified)
	w.bufferSize += int64(len(vertex.ChunkHashes) * 16)
	return nil
}

// SealBlock creates a new block from the buffered items.
func (w *DefaultDistributedWAL) SealBlock(ctx context.Context) (model.Block, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.chunks) == 0 && len(w.vertices) == 0 {
		return model.Block{}, fmt.Errorf("wal: no data to seal")
	}

	block := w.createBlock()

	// Clear buffers
	w.chunks = make([]model.SealedChunk, 0)
	w.vertices = make([]model.Vertex, 0)
	w.bufferSize = 0

	return block, nil
}

// GetBufferSize returns the current size of buffered data.
func (w *DefaultDistributedWAL) GetBufferSize() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.bufferSize
}

// Flush forces the creation of a block even if the buffer isn't full.
func (w *DefaultDistributedWAL) Flush(ctx context.Context) (model.Block, error) {
	return w.SealBlock(ctx)
}

func (w *DefaultDistributedWAL) createBlock() model.Block {
	// Serialize chunks into DataSection
	var dataSection []byte
	chunkIndex := make(map[hash.Hash]model.ChunkRegion)
	offset := uint32(0)
	for _, chunk := range w.chunks {
		length := uint32(len(chunk.EncryptedContent))
		chunkIndex[chunk.ChunkHash] = model.ChunkRegion{
			ChunkHash: chunk.ChunkHash,
			Offset:    offset,
			Length:    length,
		}
		dataSection = append(dataSection, chunk.EncryptedContent...)
		offset += length
	}

	// Serialize vertices into VertexSection (simplified)
	var vertexSection []byte
	vertexIndex := make(map[hash.Hash]model.VertexRegion)
	offset = 0
	for _, vertex := range w.vertices {
		// Simplified serialization - actual implementation would use proper encoding
		vertexBytes := vertex.Hash[:]
		length := uint32(len(vertexBytes))
		vertexIndex[vertex.Hash] = model.VertexRegion{
			VertexHash: vertex.Hash,
			Offset:     offset,
			Length:     length,
		}
		vertexSection = append(vertexSection, vertexBytes...)
		offset += length
	}

	// Calculate block hash
	hashInput := append(dataSection, vertexSection...)
	blockHash := hash.HashBytes(hashInput)

	return model.Block{
		Hash: blockHash,
		Header: model.BlockHeader{
			Version:      1,
			Created:      time.Now().UnixMilli(),
			ChunkCount:   uint32(len(w.chunks)),
			VertexCount:  uint32(len(w.vertices)),
			TotalSize:    uint32(len(dataSection) + len(vertexSection)),
		},
		DataSection:   dataSection,
		VertexSection: vertexSection,
		ChunkIndex:    chunkIndex,
		VertexIndex:   vertexIndex,
		KeyRegistry:   make(map[hash.Hash][]model.KeyEntry),
	}
}

// Ensure DefaultDistributedWAL implements the DistributedWAL interface.
var _ wal.DistributedWAL = (*DefaultDistributedWAL)(nil)

// DefaultDeletionWAL implements the DeletionWAL interface.
type DefaultDeletionWAL struct {
	mu               sync.Mutex
	pendingDeletions []hash.Hash
}

// NewDeletionWAL creates a new DefaultDeletionWAL instance.
func NewDeletionWAL() *DefaultDeletionWAL {
	return &DefaultDeletionWAL{
		pendingDeletions: make([]hash.Hash, 0),
	}
}

// LogDeletion records a hash for deletion.
func (w *DefaultDeletionWAL) LogDeletion(ctx context.Context, h hash.Hash) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.pendingDeletions = append(w.pendingDeletions, h)
	return nil
}

// ProcessDeletions processes pending deletions.
func (w *DefaultDeletionWAL) ProcessDeletions(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Implementation will actually delete the data
	// This is a placeholder for the actual implementation
	w.pendingDeletions = make([]hash.Hash, 0)
	return nil
}

// GetPendingDeletions returns the list of hashes pending deletion.
func (w *DefaultDeletionWAL) GetPendingDeletions(ctx context.Context) ([]hash.Hash, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	result := make([]hash.Hash, len(w.pendingDeletions))
	copy(result, w.pendingDeletions)
	return result, nil
}

// Ensure DefaultDeletionWAL implements the DeletionWAL interface.
var _ wal.DeletionWAL = (*DefaultDeletionWAL)(nil)
