package cas

import "github.com/i5heu/ouroboros-crypt/pkg/hash"

// SealedSlice represents a single Reed-Solomon slice (data or parity) persisted in the key-value store.
type SealedSlice struct {
	Hash            hash.Hash // Hash of the sealed slice
	RSDataSlices    uint8     // Number of data slices in the originating stripe
	RSParitySlices  uint8     // Number of parity slices in the originating stripe
	RSSliceIndex    uint8     // Index of the slice within the stripe (data slices precede parity slices)
	EncapsulatedKey []byte    // ML-KEM encapsulated secret for the sealed slice
	Nonce           []byte    // AES-GCM nonce for encryption
	Payload         []byte    // sealed slice data
}
