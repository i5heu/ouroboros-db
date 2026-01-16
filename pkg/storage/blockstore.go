package storage

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// BlockStore handles low-level Block and BlockSlice persistence.
// It is responsible for physical storage and retrieval of blocks and their shards.
type BlockStore interface {
	// StoreBlock persists a block to storage.
	StoreBlock(ctx context.Context, block model.Block) error

	// GetBlock retrieves a block by its hash.
	GetBlock(ctx context.Context, blockHash hash.Hash) (model.Block, error)

	// DeleteBlock removes a block from storage.
	DeleteBlock(ctx context.Context, blockHash hash.Hash) error

	// StoreBlockSlice persists a block slice to storage.
	StoreBlockSlice(ctx context.Context, slice model.BlockSlice) error

	// GetBlockSlice retrieves a block slice by its hash.
	GetBlockSlice(ctx context.Context, sliceHash hash.Hash) (model.BlockSlice, error)

	// ListBlockSlices returns all slices for a given block.
	ListBlockSlices(ctx context.Context, blockHash hash.Hash) ([]model.BlockSlice, error)

	// GetSealedChunkByRegion retrieves a sealed chunk using region-based lookup.
	// This allows efficient partial reads without decoding the entire block.
	GetSealedChunkByRegion(
		ctx context.Context,
		blockHash hash.Hash,
		region model.ChunkRegion,
	) (model.SealedChunk, error)

	// GetVertexByRegion retrieves a vertex using region-based lookup.
	// This allows efficient partial reads without decoding the entire block.
	GetVertexByRegion(
		ctx context.Context,
		blockHash hash.Hash,
		region model.VertexRegion,
	) (model.Vertex, error)
}
