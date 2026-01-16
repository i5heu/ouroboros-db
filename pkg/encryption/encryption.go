// Package encryption defines interfaces for encryption operations in OuroborosDB.
package encryption

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// EncryptionService handles encryption and decryption of chunks.
// It manages the Chunk <-> SealedChunk transformation.
type EncryptionService interface {
	// SealChunk encrypts a chunk for the specified public keys.
	// Returns the sealed chunk, key entries for each public key, and any error.
	SealChunk(chunk model.Chunk, pubKeys [][]byte) (model.SealedChunk, []model.KeyEntry, error)

	// UnsealChunk decrypts a sealed chunk using the provided key entry and private key.
	UnsealChunk(sealed model.SealedChunk, keyEntry model.KeyEntry, privKey []byte) (model.Chunk, error)

	// GenerateKeyEntry creates a key entry for a chunk, allowing a specific public key
	// to decrypt it using the provided AES key.
	GenerateKeyEntry(chunkHash hash.Hash, pubKey []byte, aesKey []byte) (model.KeyEntry, error)
}
