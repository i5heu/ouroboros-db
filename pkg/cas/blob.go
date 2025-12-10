package cas

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Blob struct {
	Key     string    // Key like a path or name, this is non-unique
	Hash    hash.Hash // Key is derived from all fields except Children; must be zero when writing new data because it is generated from the content
	Parent  hash.Hash // Key of the parent value
	Created int64     // Unix timestamp when the data was created
}

// KvData represents the `Data` structure with full SliceRecords for decryption and reconstruction.
type KvData struct {
	Key             hash.Hash
	Slices          []SealedSlice // Content RSSlices
	ChunkHashes     []hash.Hash   // Order of content chunk hashes
	MetaSlices      []SealedSlice // Metadata RSSlices
	MetaChunkHashes []hash.Hash   // Order of metadata chunk hashes
	Parent          hash.Hash     // Key of the parent chunk
	Children        []hash.Hash   // Keys of the child chunks
	Created         int64         // Unix timestamp when the data was created
	Aliases         []hash.Hash   // Aliases for the data
	ContentType     string        // Content type for the data - IETF RFC 9110 §8.3 https://datatracker.ietf.org/doc/html/rfc9110#section-8.3 (unencrypted)
}

// SealedSlice represents a single Reed-Solomon slice (data or parity) persisted in the key-value store.
type SealedSlice struct {
	ChunkHash       hash.Hash // Hash of the clear chunk produced by Buzhash
	SealedHash      hash.Hash // Hash of the sealed chunk (compressed + encrypted) prior to Reed-Solomon encoding
	RSDataSlices    uint8     // Number of data slices in the originating stripe
	RSParitySlices  uint8     // Number of parity slices in the originating stripe
	RSSliceIndex    uint8     // Index of the slice within the stripe (data slices precede parity slices)
	Size            uint64    // Size of this slice payload in bytes
	OriginalSize    uint64    // Size of the sealed chunk before Reed-Solomon encoding
	EncapsulatedKey []byte    // ML-KEM encapsulated secret for the sealed chunk
	Nonce           []byte    // AES-GCM nonce for encryption
	Payload         []byte    // Slice payload after Reed-Solomon encoding
}
