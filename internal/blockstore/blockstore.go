// Package blockstore provides block storage implementations for OuroborosDB.
package blockstore

import (
	"context"
	"fmt"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/i5heu/ouroboros-db/pkg/storage"
)

// DefaultBlockStore implements the BlockStore interface using in-memory
// storage. This is a reference implementation; production should use a
// persistent backend.
type DefaultBlockStore struct {
	mu     sync.RWMutex
	blocks map[hash.Hash]model.Block
	slices map[hash.Hash]model.BlockSlice
}

// NewBlockStore creates a new DefaultBlockStore instance.
func NewBlockStore() *DefaultBlockStore {
	return &DefaultBlockStore{
		blocks: make(map[hash.Hash]model.Block),
		slices: make(map[hash.Hash]model.BlockSlice),
	}
}

// StoreBlock persists a block to storage.
func (s *DefaultBlockStore) StoreBlock(
	ctx context.Context,
	block model.Block,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if block.Hash == (hash.Hash{}) {
		return fmt.Errorf("blockstore: block hash is required")
	}

	s.blocks[block.Hash] = block
	return nil
}

// GetBlock retrieves a block by its hash.
func (s *DefaultBlockStore) GetBlock(
	ctx context.Context,
	blockHash hash.Hash,
) (model.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, exists := s.blocks[blockHash]
	if !exists {
		return model.Block{}, fmt.Errorf(
			"blockstore: block %s not found",
			blockHash,
		)
	}
	return block, nil
}

// DeleteBlock removes a block from storage.
func (s *DefaultBlockStore) DeleteBlock(
	ctx context.Context,
	blockHash hash.Hash,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.blocks, blockHash)
	return nil
}

// StoreBlockSlice persists a block slice to storage.
func (s *DefaultBlockStore) StoreBlockSlice(
	ctx context.Context,
	slice model.BlockSlice,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if slice.Hash == (hash.Hash{}) {
		return fmt.Errorf("blockstore: slice hash is required")
	}

	s.slices[slice.Hash] = slice
	return nil
}

// GetBlockSlice retrieves a block slice by its hash.
func (s *DefaultBlockStore) GetBlockSlice(
	ctx context.Context,
	sliceHash hash.Hash,
) (model.BlockSlice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	slice, exists := s.slices[sliceHash]
	if !exists {
		return model.BlockSlice{}, fmt.Errorf(
			"blockstore: slice %s not found",
			sliceHash,
		)
	}
	return slice, nil
}

// ListBlockSlices returns all slices for a given block.
func (s *DefaultBlockStore) ListBlockSlices(
	ctx context.Context,
	blockHash hash.Hash,
) ([]model.BlockSlice, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var slices []model.BlockSlice
	for _, slice := range s.slices {
		if slice.BlockHash == blockHash {
			slices = append(slices, slice)
		}
	}
	return slices, nil
}

// GetSealedChunkByRegion retrieves a sealed chunk using region-based lookup.
func (s *DefaultBlockStore) GetSealedChunkByRegion(
	ctx context.Context,
	blockHash hash.Hash,
	region model.ChunkRegion,
) (model.SealedChunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, exists := s.blocks[blockHash]
	if !exists {
		return model.SealedChunk{}, fmt.Errorf(
			"blockstore: block %s not found",
			blockHash,
		)
	}

	// Extract chunk data from block's DataSection using region
	if int(region.Offset+region.Length) > len(block.DataSection) {
		return model.SealedChunk{}, fmt.Errorf(
			"blockstore: region exceeds block data section",
		)
	}

	// This is a simplified implementation - actual implementation would
	// deserialize the SealedChunk from the bytes
	return model.SealedChunk{
		ChunkHash:        region.ChunkHash,
		EncryptedContent: block.DataSection[region.Offset : region.Offset+region.Length],
	}, nil
}

// GetVertexByRegion retrieves a vertex using region-based lookup.
func (s *DefaultBlockStore) GetVertexByRegion(
	ctx context.Context,
	blockHash hash.Hash,
	region model.VertexRegion,
) (model.Vertex, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	block, exists := s.blocks[blockHash]
	if !exists {
		return model.Vertex{}, fmt.Errorf(
			"blockstore: block %s not found",
			blockHash,
		)
	}

	// Extract vertex data from block's VertexSection using region
	if int(region.Offset+region.Length) > len(block.VertexSection) {
		return model.Vertex{}, fmt.Errorf(
			"blockstore: region exceeds block vertex section",
		)
	}

	// This is a simplified implementation - actual implementation would
	// deserialize the Vertex from the bytes
	return model.Vertex{
		Hash: region.VertexHash,
	}, nil
}

// Ensure DefaultBlockStore implements the BlockStore interface.
var _ storage.BlockStore = (*DefaultBlockStore)(nil)
