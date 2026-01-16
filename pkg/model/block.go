package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// BlockHeader contains global block settings and metadata.
//
// The header provides essential information about the block's structure
// and encoding parameters, allowing readers to correctly parse and
// reconstruct the block contents.
type BlockHeader struct {
	// Version is the block format version.
	// This allows for future format changes while maintaining
	// backward compatibility.
	Version uint8

	// Created is the Unix timestamp (milliseconds) when the block was created.
	// This is used for ordering and garbage collection.
	Created int64

	// RSDataSlices is the number of Reed-Solomon data slices (k).
	// The block can be reconstructed from any k slices.
	RSDataSlices uint8

	// RSParitySlices is the number of Reed-Solomon parity slices (p).
	// The block can tolerate up to p missing slices.
	RSParitySlices uint8

	// ChunkCount is the number of sealed chunks in this block's DataSection.
	ChunkCount uint32

	// VertexCount is the number of vertices in this block's VertexSection.
	VertexCount uint32

	// TotalSize is the total size of the block in bytes (all sections combined).
	TotalSize uint32
}

// Block is the central archive structure for persistent storage.
//
// A Block is the fundamental unit of persistent storage in OuroborosDB.
// It aggregates multiple SealedChunks and Vertices into a single archive
// that can be efficiently stored, replicated, and distributed across the cluster.
//
// # Block Structure
//
// A Block contains several sections:
//
//   - Header: Metadata about the block (version, timestamps, counts)
//   - DataSection: Serialized SealedChunks (encrypted content)
//   - VertexSection: Serialized Vertices (unencrypted metadata)
//   - ChunkIndex: Maps ChunkHash → byte range in DataSection
//   - VertexIndex: Maps VertexHash → byte range in VertexSection
//   - KeyRegistry: Maps ChunkHash → KeyEntry list (access control)
//
// # Block Creation
//
// Blocks are created by the DistributedWAL when the buffer reaches the
// target size (typically 16MB). The SealBlock() operation:
//  1. Serializes buffered SealedChunks into DataSection
//  2. Serializes buffered Vertices into VertexSection
//  3. Builds the ChunkIndex and VertexIndex
//  4. Computes the block Hash
//  5. Returns the complete Block
//
// # Distribution
//
// Once created, Blocks are:
//  1. Split into BlockSlices using Reed-Solomon encoding
//  2. Distributed across cluster nodes via the DataRouter
//  3. Each node stores a subset of slices for redundancy
//
// # Efficient Access
//
// The ChunkIndex and VertexIndex enable region-based access, allowing
// retrieval of individual chunks or vertices without loading the entire
// block into memory. This is critical for performance with large blocks.
//
// # Hash Computation
//
// The Block Hash is computed from:
//   - Header (serialized)
//   - DataSection
//   - VertexSection
//
// This provides integrity verification for the entire block.
type Block struct {
	// Hash is the unique identifier for this block, derived from its contents.
	Hash hash.Hash

	// Header contains the block's metadata and encoding parameters.
	Header BlockHeader

	// DataSection contains the serialized SealedChunks (encrypted content).
	// Use ChunkIndex to locate specific chunks within this section.
	DataSection []byte

	// VertexSection contains the serialized Vertices (unencrypted metadata).
	// Use VertexIndex to locate specific vertices within this section.
	VertexSection []byte

	// KeyRegistry maps chunk hashes to their key entries for access control.
	KeyRegistry map[hash.Hash][]KeyEntry

	// ChunkIndex maps chunk hashes to their byte region in the DataSection.
	// This enables efficient partial reads of individual chunks.
	ChunkIndex map[hash.Hash]ChunkRegion

	// VertexIndex maps vertex hashes to their byte region in the VertexSection.
	// This enables efficient partial reads of individual vertices.
	VertexIndex map[hash.Hash]VertexRegion
}

// ChunkRegion specifies the byte range of a SealedChunk within a Block's DataSection.
//
// ChunkRegion enables efficient partial reads by providing the exact byte
// offset and length of a specific chunk within the DataSection. This allows
// the BlockStore to seek directly to the chunk data without parsing the
// entire block.
//
// # Usage
//
//	region := block.ChunkIndex[chunkHash]
//	chunkData := block.DataSection[region.Offset : region.Offset+region.Length]
type ChunkRegion struct {
	// ChunkHash identifies which chunk this region locates.
	ChunkHash hash.Hash

	// Offset is the byte offset from the start of DataSection.
	Offset uint32

	// Length is the number of bytes for this chunk's serialized data.
	Length uint32
}

// VertexRegion specifies the byte range of a Vertex within a Block's VertexSection.
//
// VertexRegion enables efficient partial reads of vertex metadata without
// loading the entire block. This is particularly useful for DAG traversal
// operations that need to read many vertices.
//
// # Usage
//
//	region := block.VertexIndex[vertexHash]
//	vertexData := block.VertexSection[region.Offset : region.Offset+region.Length]
type VertexRegion struct {
	// VertexHash identifies which vertex this region locates.
	VertexHash hash.Hash

	// Offset is the byte offset from the start of VertexSection.
	Offset uint32

	// Length is the number of bytes for this vertex's serialized data.
	Length uint32
}
