package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// KeyEntry represents the Access Control unit.
// It explicitly links a User (PubKey) to Content (ChunkHash).
// Note: The Chunk must not be in the same Block as its KeyEntry.
type KeyEntry struct {
	// ChunkHash identifies which chunk this key entry unlocks.
	ChunkHash hash.Hash

	// PubKeyHash is the hash of the public key that can use this entry.
	PubKeyHash hash.Hash

	// EncapsulatedAESKey is the AES key encapsulated for the specific public key.
	EncapsulatedAESKey []byte
}
