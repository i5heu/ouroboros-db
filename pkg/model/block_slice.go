package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// BlockSlice represents a physical shard of a Block.
// Blocks are split into slices using Reed-Solomon encoding for
// redundancy and distribution across nodes.
type BlockSlice struct {
	// Hash is the unique identifier for this slice.
	Hash hash.Hash

	// BlockHash identifies the parent block this slice belongs to.
	BlockHash hash.Hash

	// RSSliceIndex is the index of this slice within the Reed-Solomon stripe.
	// Data slice indices precede parity slice indices.
	// Example: for k=4 data and p=3 parity slices,
	// data indices are 0,1,2,3 and parity indices are 4,5,6.
	RSSliceIndex uint8

	// RSDataSlices is the number of data slices (k) in the encoding.
	RSDataSlices uint8

	// RSParitySlices is the number of parity slices (p) in the encoding.
	RSParitySlices uint8

	// Payload is the actual slice data.
	Payload []byte
}
