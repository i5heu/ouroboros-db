package model

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// KeyEntry represents the Access Control unit for encrypted content.
//
// KeyEntry is the mechanism that grants access to encrypted content. It
// explicitly links a User (identified by PubKeyHash) to Content (identified
// by ChunkHash) by storing the AES encryption key in a form that only the
// specified user can decrypt.
//
// # Key Encapsulation
//
// The EncapsulatedAESKey field contains the AES-256-GCM key used to encrypt
// the chunk, wrapped using ML-KEM (post-quantum key encapsulation) for
// the recipient's public key. Only the holder of the corresponding
// private key can recover the AES key and decrypt the content.
//
// # Access Grant Flow
//
//  1. Content is encrypted with a random AES-256-GCM key
//  2. For each authorized user, a KeyEntry is created:
//     - ChunkHash: identifies the encrypted content
//     - PubKeyHash: identifies the authorized user
//     - EncapsulatedAESKey: AES key wrapped for that user's public key
//  3. KeyEntry is stored separately from the encrypted content
//
// # Decryption Flow
//
//  1. Retrieve the SealedChunk by ChunkHash
//  2. Request KeyEntry for (ChunkHash, own PubKeyHash)
//  3. Decapsulate the AES key using own private key
//  4. Decrypt the SealedChunk content with the AES key
//
// # Key Revocation
//
// Access can be revoked by deleting the KeyEntry. The encrypted content
// remains unchanged, but the user can no longer obtain the decryption key.
// Note that users who previously decrypted the content may have cached it.
type KeyEntry struct {
	// ChunkHash identifies which chunk this key entry unlocks.
	// This must match the ChunkHash of the SealedChunk that the
	// recipient wants to decrypt.
	ChunkHash hash.Hash

	// PubKeyHash is the hash of the public key that can use this entry.
	// Only the holder of the corresponding private key can decapsulate
	// the EncapsulatedAESKey.
	PubKeyHash hash.Hash

	// EncapsulatedAESKey is the AES-256-GCM key encapsulated using ML-KEM
	// for the public key identified by PubKeyHash. The encapsulated
	// key can only be recovered using the corresponding private key.
	EncapsulatedAESKey []byte
}
