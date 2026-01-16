package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// BlockHeader contains global block settings and metadata.
type BlockHeader struct {
	// Version is the block format version.
	Version uint8

	// Created is the Unix timestamp when the block was created.
	Created int64

	// RSDataSlices is the number of Reed-Solomon data slices (k).
	RSDataSlices uint8

	// RSParitySlices is the number of Reed-Solomon parity slices (p).
	RSParitySlices uint8

	// ChunkCount is the number of sealed chunks in this block.
	ChunkCount uint32

	// VertexCount is the number of vertices in this block.
	VertexCount uint32

	// TotalSize is the total size of the block in bytes.
	TotalSize uint32
}

// Block is the central archive structure that contains sealed chunks,
// vertices, and key entries.
type Block struct {
	// Hash is the unique identifier for this block.
	Hash hash.Hash

	// Header contains the block's metadata.
	Header BlockHeader

	// DataSection contains the serialized sealed chunks.
	DataSection []byte

	// VertexSection contains the serialized vertices.
	VertexSection []byte

	// KeyRegistry maps chunk hashes to their key entries.
	KeyRegistry map[hash.Hash][]KeyEntry

	// ChunkIndex maps chunk hashes to their region in the DataSection.
	ChunkIndex map[hash.Hash]ChunkRegion

	// VertexIndex maps vertex hashes to their region in the VertexSection.
	VertexIndex map[hash.Hash]VertexRegion
}

// ChunkRegion specifies the byte range of a SealedChunk within a Block's DataSection.
// This allows for efficient partial reads without decoding the entire block.
type ChunkRegion struct {
	// ChunkHash identifies which chunk this region locates.
	ChunkHash hash.Hash

	// Offset is the byte offset within the DataSection.
	Offset uint32

	// Length is the number of bytes for this chunk.
	Length uint32
}

// VertexRegion specifies the byte range of a Vertex within a Block's VertexSection.
// This allows for efficient partial reads without decoding the entire block.
type VertexRegion struct {
	// VertexHash identifies which vertex this region locates.
	VertexHash hash.Hash

	// Offset is the byte offset within the VertexSection.
	Offset uint32

	// Length is the number of bytes for this vertex.
	Length uint32
}
