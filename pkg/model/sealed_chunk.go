package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// SealedChunk represents an encrypted payload ready for persistent storage.
//
// A SealedChunk is the encrypted form of a Chunk. It contains the encrypted
// content along with metadata needed for integrity verification and decryption.
// SealedChunks are stored in the DataSection of Blocks.
//
// # Encryption Process
//
// The transformation from Chunk to SealedChunk involves:
//  1. Compress the chunk content (zstd)
//  2. Generate a random AES-256 key
//  3. Encrypt with AES-GCM using the generated key
//  4. Encapsulate the AES key for each authorized user (ML-KEM)
//  5. Store the encapsulated keys as KeyEntry records
//
// # Integrity Verification
//
// SealedChunk provides two levels of integrity:
//   - ChunkHash: verifies the original cleartext (after decryption)
//   - SealedHash: verifies the encrypted content (without decryption)
//
// The SealedHash allows nodes to verify data integrity during replication
// and storage without having access to decryption keys.
//
// # Access Control
//
// Access to a SealedChunk is controlled through KeyEntry records. Each
// KeyEntry contains the AES key encapsulated for a specific user's public key.
//
// # Storage Location
//
// SealedChunks are stored in the DataSection of Blocks. The ChunkRegion
// index maps ChunkHash to the byte offset and length within the DataSection,
// enabling efficient partial reads.
type SealedChunk struct {
	// ChunkHash is the hash of the original cleartext chunk.
	// This is used to verify content integrity after decryption
	// and to link SealedChunks to their Vertex references.
	ChunkHash hash.Hash

	// SealedHash is the hash of the encrypted content.
	// This enables integrity verification of the encrypted data
	// without requiring decryption keys, useful for storage
	// verification and replication.
	SealedHash hash.Hash

	// EncryptedContent is the AES-GCM encrypted chunk data.
	// The content is compressed before encryption.
	EncryptedContent []byte

	// Nonce is the AES-GCM nonce (12 bytes) used for encryption.
	// Each encryption operation must use a unique nonce.
	Nonce []byte

	// OriginalSize is the size of the original cleartext content in bytes.
	// This is stored to enable pre-allocation during decryption and
	// to verify successful decompression.
	OriginalSize int
}
