// Package encryption defines interfaces for encryption operations in OuroborosDB.
//
// This package provides the EncryptionService interface that handles the
// transformation between cleartext Chunks and encrypted SealedChunks. It is
// a core component of the storage pipeline, ensuring all content is encrypted
// before persistent storage.
//
// # Encryption Architecture
//
// OuroborosDB uses a hybrid encryption scheme:
//
//  1. Content Encryption: AES-256-GCM for symmetric encryption of chunk data
//  2. Key Encapsulation: ML-KEM (post-quantum) for wrapping AES keys
//
// This provides:
//   - Fast symmetric encryption for large content
//   - Post-quantum security for key distribution
//   - Multi-recipient support (same content, multiple authorized users)
//
// # Encryption Flow (SealChunk)
//
//  1. Compress the chunk content (zstd)
//  2. Generate a random 256-bit AES key
//  3. Generate a random 96-bit nonce
//  4. Encrypt with AES-256-GCM
//  5. For each authorized public key:
//     - Encapsulate the AES key using ML-KEM
//     - Create a KeyEntry linking (ChunkHash, PubKeyHash, EncapsulatedKey)
//  6. Return SealedChunk and KeyEntry list
//
// # Decryption Flow (UnsealChunk)
//
//  1. Retrieve the KeyEntry for the user's public key
//  2. Decapsulate the AES key using the user's private key
//  3. Decrypt with AES-256-GCM using the AES key and nonce
//  4. Decompress the content
//  5. Verify ChunkHash matches the decrypted content
//  6. Return the cleartext Chunk
//
// # Security Considerations
//
//   - AES keys are never stored in cleartext
//   - Each chunk uses a unique AES key and nonce
//   - ML-KEM provides post-quantum security
//   - Compression before encryption prevents some attacks
package encryption

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// EncryptionService handles encryption and decryption of chunks.
//
// EncryptionService is responsible for the Chunk ↔ SealedChunk transformation.
// It encapsulates all cryptographic operations and key management required
// for secure content storage.
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Multiple goroutines may
// encrypt and decrypt chunks simultaneously.
//
// # Error Handling
//
// Methods return errors for:
//   - Invalid input (nil chunk, empty public keys)
//   - Cryptographic failures (invalid key, corrupted data)
//   - Hash verification failures (content tampering)
type EncryptionService interface {
	// SealChunk encrypts a chunk for the specified public keys.
	//
	// This is the main encryption entry point. It:
	//  1. Compresses the chunk content
	//  2. Generates a random AES key and nonce
	//  3. Encrypts with AES-256-GCM
	//  4. Creates a KeyEntry for each authorized public key
	//
	// Parameters:
	//   - chunk: The cleartext chunk to encrypt
	//   - pubKeys: List of public keys that should have access
	//
	// Returns:
	//   - SealedChunk: The encrypted chunk with metadata
	//   - []KeyEntry: One KeyEntry per public key (same order as pubKeys)
	//   - error: If encryption fails
	//
	// Example:
	//
	//	sealed, entries, err := enc.SealChunk(chunk, [][]byte{user1PubKey, user2PubKey})
	//	// sealed contains encrypted data
	//	// entries[0] grants access to user1
	//	// entries[1] grants access to user2
	SealChunk(chunk model.Chunk, pubKeys [][]byte) (model.SealedChunk, []model.KeyEntry, error)

	// UnsealChunk decrypts a sealed chunk using the provided key entry and private key.
	//
	// This is the main decryption entry point. It:
	//  1. Decapsulates the AES key using the private key
	//  2. Decrypts with AES-256-GCM
	//  3. Decompresses the content
	//  4. Verifies the ChunkHash matches
	//
	// Parameters:
	//   - sealed: The encrypted chunk to decrypt
	//   - keyEntry: The KeyEntry granting access (must match user's public key)
	//   - privKey: The user's private key for decapsulation
	//
	// Returns:
	//   - Chunk: The decrypted cleartext chunk
	//   - error: If decryption fails (invalid key, corrupted data, hash mismatch)
	//
	// The keyEntry.PubKeyHash must correspond to the privKey. If they don't
	// match, decapsulation will fail.
	//
	// Example:
	//
	//	chunk, err := enc.UnsealChunk(sealed, myKeyEntry, myPrivateKey)
	//	if err != nil {
	//	    // Access denied or corrupted data
	//	}
	UnsealChunk(sealed model.SealedChunk, keyEntry model.KeyEntry, privKey []byte) (model.Chunk, error)

	// GenerateKeyEntry creates a key entry for granting access to encrypted content.
	//
	// This is used to grant access to existing encrypted content without
	// re-encrypting. Given the AES key used to encrypt a chunk, it creates
	// a new KeyEntry for an additional user.
	//
	// Parameters:
	//   - chunkHash: The hash of the chunk being shared
	//   - pubKey: The public key of the new recipient
	//   - aesKey: The AES key used to encrypt the chunk
	//
	// Returns:
	//   - KeyEntry: The new access grant for the recipient
	//   - error: If key encapsulation fails
	//
	// Use case: Sharing previously encrypted content with a new user:
	//
	//	// Original user has the AES key from their KeyEntry decapsulation
	//	newEntry, err := enc.GenerateKeyEntry(chunkHash, newUserPubKey, aesKey)
	//	// Store newEntry to grant access to newUser
	//
	// Note: The caller must have access to the original AES key, which
	// typically requires decrypting a chunk first.
	GenerateKeyEntry(chunkHash hash.Hash, pubKey []byte, aesKey []byte) (model.KeyEntry, error)
}
