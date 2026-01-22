package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// BlockSlice represents a physical shard of a Block for distributed storage.
//
// Blocks are split into BlockSlices using Reed-Solomon erasure coding, which
// provides redundancy and fault tolerance. The encoding parameters (k data
// slices, p parity slices) determine the redundancy level:
//
//   - k: minimum slices needed to reconstruct the block
//   - p: number of additional parity slices
//   - n = k + p: total slices generated
//
// The block can be reconstructed from any k slices, meaning up to p slices
// can be lost without data loss.
//
// # Distribution Strategy
//
// BlockSlices are distributed across cluster nodes to maximize availability:
//   - Each node stores a subset of slices for each block
//   - Slices are placed to maximize geographic/failure domain diversity
//   - The DataRouter coordinates slice distribution and retrieval
//
// # Reconstruction
//
// To reconstruct a Block:
//  1. Retrieve at least k distinct slices (by RSSliceIndex)
//  2. Use Reed-Solomon decoding to reconstruct the original block data
//  3. Parse the reconstructed data into Block structure
//
// # Index Ordering
//
// The RSSliceIndex follows Reed-Solomon conventions:
//   - Indices 0 to k-1: data slices (contain actual block data)
//   - Indices k to k+p-1: parity slices (contain error correction data)
//
// # Example
//
// For a block with k=4 data slices and p=3 parity slices:
//   - Total slices: 7 (indices 0-6)
//   - Data slices: indices 0, 1, 2, 3
//   - Parity slices: indices 4, 5, 6
//   - Reconstruction requires any 4 slices
//   - Can tolerate loss of up to 3 slices
type BlockSlice struct {
	// Hash is the unique identifier for this slice, derived from its contents.
	// Computed from: BlockHash, RSSliceIndex, RSDataSlices, RSParitySlices,
	// Payload
	Hash hash.Hash

	// BlockHash identifies the parent block this slice belongs to.
	// All slices with the same BlockHash can be used together for reconstruction.
	BlockHash hash.Hash

	// RSSliceIndex is the index of this slice within the Reed-Solomon stripe.
	// - Values 0 to RSDataSlices-1 are data slices
	// - Values RSDataSlices to RSDataSlices+RSParitySlices-1 are parity slices
	RSSliceIndex uint8

	// RSDataSlices is the number of data slices (k) in the encoding.
	// This is the minimum number of slices needed for reconstruction.
	RSDataSlices uint8

	// RSParitySlices is the number of parity slices (p) in the encoding.
	// This determines how many slices can be lost without data loss.
	RSParitySlices uint8

	// Payload is the actual slice data (either data or parity bytes).
	// All slices for a block have the same payload length.
	Payload []byte

	// OriginalSize is the size in bytes of the serialized Block that was
	// erasure-encoded into these slices. This is required by the
	// Reed-Solomon join operation to trim padding added during encoding.
	OriginalSize uint64
}
