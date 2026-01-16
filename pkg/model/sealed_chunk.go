package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// SealedChunk represents an encrypted payload.
// It includes a SealedHash for integrity checking of the encrypted data
// without needing decryption keys.
type SealedChunk struct {
	// ChunkHash is the hash of the original cleartext chunk.
	ChunkHash hash.Hash

	// SealedHash is the hash of the encrypted content for integrity verification.
	SealedHash hash.Hash

	// EncryptedContent is the encrypted chunk data.
	EncryptedContent []byte

	// Nonce is the AES-GCM nonce used for encryption.
	Nonce []byte

	// OriginalSize is the size of the original cleartext content.
	OriginalSize int
}
