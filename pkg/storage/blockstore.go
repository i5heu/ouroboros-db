package storage

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// BlockStore handles low-level Block and BlockSlice persistence.
//
// BlockStore is the physical storage layer responsible for persisting and
// retrieving Blocks and their Reed-Solomon encoded BlockSlices. It provides
// both full block operations and efficient region-based partial reads.
//
// # Responsibilities
//
//   - Persist complete Blocks to storage
//   - Persist and manage BlockSlices for distribution
//   - Provide region-based access to SealedChunks and Vertices
//   - Handle block reconstruction from slices (when needed)
//
// # Storage Model
//
// BlockStore manages two types of data:
//
//  1. Complete Blocks: The full archive structure with all sections
//  2. BlockSlices: Reed-Solomon shards for distributed storage
//
// In a distributed deployment, nodes typically store BlockSlices rather than
// complete Blocks. The DataRouter coordinates which slices each node stores.
//
// # Region-Based Access
//
// The GetSealedChunkByRegion and GetVertexByRegion methods enable efficient
// partial reads. Instead of loading an entire block to access one chunk:
//
//  1. Query the block's index to get the ChunkRegion/VertexRegion
//  2. Call the region-based method with the block hash and region
//  3. BlockStore seeks directly to the byte range and returns the item
//
// This is critical for performance with large blocks (e.g., 16MB).
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Multiple goroutines may
// read and write blocks simultaneously.
//
// # Error Handling
//
// Methods return errors for:
//   - Block/slice not found
//   - Storage corruption (hash mismatch)
//   - I/O failures
//   - Invalid regions (out of bounds)
type BlockStore interface {
	// StoreBlock persists a complete block to storage.
	//
	// The block is stored with its full structure including all sections,
	// indices, and the key registry. The block's Hash is used as the
	// storage key.
	//
	// Parameters:
	//   - block: The complete block to store
	//
	// Returns:
	//   - Error if storage fails
	//
	// Implementations should verify the block hash matches its contents
	// before storing.
	StoreBlock(ctx context.Context, block model.Block) error

	// GetBlock retrieves a complete block by its hash.
	//
	// Returns the full block structure with all sections and indices.
	// If the block is stored as slices, this may trigger reconstruction.
	//
	// Parameters:
	//   - blockHash: The hash of the block to retrieve
	//
	// Returns:
	//   - The complete block
	//   - Error if the block doesn't exist or reconstruction fails
	GetBlock(ctx context.Context, blockHash hash.Hash) (model.Block, error)

	// DeleteBlock removes a block from storage.
	//
	// This deletes the complete block. If the block is stored as slices,
	// all slices for this block should also be deleted.
	//
	// Parameters:
	//   - blockHash: The hash of the block to delete
	//
	// Returns:
	//   - Error if deletion fails (not found is typically not an error)
	DeleteBlock(ctx context.Context, blockHash hash.Hash) error

	// StoreBlockSlice persists a single Reed-Solomon slice.
	//
	// BlockSlices are the unit of distribution in the cluster. Each node
	// stores a subset of slices for each block.
	//
	// Parameters:
	//   - slice: The block slice to store
	//
	// Returns:
	//   - Error if storage fails
	StoreBlockSlice(ctx context.Context, slice model.BlockSlice) error

	// GetBlockSlice retrieves a block slice by its hash.
	//
	// Parameters:
	//   - sliceHash: The hash of the slice to retrieve
	//
	// Returns:
	//   - The block slice with its payload
	//   - Error if the slice doesn't exist
	GetBlockSlice(ctx context.Context, sliceHash hash.Hash) (model.BlockSlice, error)

	// ListBlockSlices returns all locally stored slices for a given block.
	//
	// This returns only the slices stored on this node. In a distributed
	// deployment, a node typically stores a subset of all slices.
	//
	// Parameters:
	//   - blockHash: The hash of the parent block
	//
	// Returns:
	//   - Slice of BlockSlices stored locally (may not be all slices)
	//   - Error if the lookup fails
	ListBlockSlices(ctx context.Context, blockHash hash.Hash) ([]model.BlockSlice, error)

	// GetSealedChunkByRegion retrieves a sealed chunk using region-based lookup.
	//
	// This method enables efficient partial reads by seeking directly to the
	// chunk's byte range within the block's DataSection, without loading the
	// entire block into memory.
	//
	// Parameters:
	//   - blockHash: The hash of the block containing the chunk
	//   - region: The ChunkRegion specifying the byte offset and length
	//
	// Returns:
	//   - The deserialized SealedChunk
	//   - Error if the block doesn't exist or region is invalid
	//
	// The region should be obtained from the block's ChunkIndex.
	GetSealedChunkByRegion(
		ctx context.Context,
		blockHash hash.Hash,
		region model.ChunkRegion,
	) (model.SealedChunk, error)

	// GetVertexByRegion retrieves a vertex using region-based lookup.
	//
	// This method enables efficient partial reads by seeking directly to the
	// vertex's byte range within the block's VertexSection, without loading
	// the entire block into memory.
	//
	// Parameters:
	//   - blockHash: The hash of the block containing the vertex
	//   - region: The VertexRegion specifying the byte offset and length
	//
	// Returns:
	//   - The deserialized Vertex
	//   - Error if the block doesn't exist or region is invalid
	//
	// The region should be obtained from the block's VertexIndex.
	GetVertexByRegion(
		ctx context.Context,
		blockHash hash.Hash,
		region model.VertexRegion,
	) (model.Vertex, error)
}
