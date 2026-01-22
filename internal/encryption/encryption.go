// Package encryption provides encryption service implementations for
// OuroborosDB.
package encryption

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/encryption"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/klauspost/compress/zstd"
)

const (
	// aesKeySize is the size of AES-256 keys in bytes.
	aesKeySize = 32

	// nonceSize is the standard AES-GCM nonce size in bytes.
	nonceSize = 12

	// mlkemEncapsulatedKeySize is the size of ML-KEM-1024 encapsulated keys.
	mlkemEncapsulatedKeySize = 1568

	// mlkemPublicKeySize is the size of ML-KEM-1024 public keys.
	mlkemPublicKeySize = 1568

	// mldsaPublicKeySize is the size of ML-DSA-87 public keys.
	mldsaPublicKeySize = 2592

	// mlkemPrivateKeySize is the size of ML-KEM-1024 private keys.
	mlkemPrivateKeySize = 3168

	// mldsaPrivateKeySize is the size of ML-DSA-87 private keys.
	mldsaPrivateKeySize = 4896
)

// DefaultEncryptionService implements the EncryptionService interface
// using ouroboros-crypt for post-quantum encryption.
type DefaultEncryptionService struct{}

// NewEncryptionService creates a new DefaultEncryptionService instance.
func NewEncryptionService() *DefaultEncryptionService {
	return &DefaultEncryptionService{}
}

// SealChunk encrypts a chunk for the specified public keys.
//
// The encryption process:
//  1. Compress content with zstd
//  2. Generate random AES-256 key
//  3. Generate random 12-byte nonce
//  4. Encrypt with AES-256-GCM
//  5. For each public key, encapsulate the AES key with ML-KEM
//
// pubKeys should contain serialized public keys. Each key should be either:
//   - KEM-only: 1568 bytes (ML-KEM-1024 public key)
//   - Full key: 4160 bytes (ML-KEM + ML-DSA public keys concatenated)
func (s *DefaultEncryptionService) SealChunk(
	chunk model.Chunk,
	pubKeys [][]byte,
) (model.SealedChunk, []model.KeyEntry, error) {
	if len(chunk.Content) == 0 {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: chunk content is empty",
		)
	}

	if len(pubKeys) == 0 {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: at least one public key is required",
		)
	}

	// Step 1: Compress content with zstd
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: creating zstd encoder: %w",
			err,
		)
	}
	defer func() { _ = encoder.Close() }()
	compressed := encoder.EncodeAll(chunk.Content, nil)

	// Step 2: Generate random AES-256 key
	aesKey := make([]byte, aesKeySize)
	if _, err := rand.Read(aesKey); err != nil {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: generating AES key: %w",
			err,
		)
	}

	// Step 3: Generate random nonce
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: generating nonce: %w",
			err,
		)
	}

	// Step 4: Encrypt with AES-256-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: creating AES cipher: %w",
			err,
		)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return model.SealedChunk{}, nil, fmt.Errorf(
			"encryption: creating GCM: %w",
			err,
		)
	}
	ciphertext := aead.Seal(nil, nonce, compressed, nil)

	// Step 5: Create KeyEntry for each public key
	keyEntries := make([]model.KeyEntry, 0, len(pubKeys))
	for _, pubKeyBytes := range pubKeys {
		keyEntry, err := s.GenerateKeyEntry(chunk.Hash, pubKeyBytes, aesKey)
		if err != nil {
			return model.SealedChunk{}, nil, fmt.Errorf(
				"encryption: generating key entry: %w",
				err,
			)
		}
		keyEntries = append(keyEntries, keyEntry)
	}

	// Compute hash of encrypted content
	sealedHash := hash.HashBytes(ciphertext)

	sealed := model.SealedChunk{
		ChunkHash:        chunk.Hash,
		SealedHash:       sealedHash,
		EncryptedContent: ciphertext,
		Nonce:            nonce,
		OriginalSize:     chunk.Size,
	}

	return sealed, keyEntries, nil
}

// UnsealChunk decrypts a sealed chunk using the provided key entry and private
// key.
//
// The decryption process:
//  1. Decapsulate to recover the AES key from the KeyEntry
//  2. Decrypt with AES-256-GCM
//  3. Decompress with zstd
//  4. Verify the ChunkHash matches the decrypted content
//
// privKey should be the serialized private key, either:
//   - KEM-only: 3168 bytes (ML-KEM-1024 private key)
//   - Full key: 8064 bytes (ML-KEM + ML-DSA private keys concatenated)
func (s *DefaultEncryptionService) UnsealChunk(
	sealed model.SealedChunk,
	keyEntry model.KeyEntry,
	privKey []byte,
) (model.Chunk, error) {
	if len(sealed.EncryptedContent) == 0 {
		return model.Chunk{}, fmt.Errorf("decryption: encrypted content is empty")
	}

	if len(keyEntry.EncapsulatedAESKey) < mlkemEncapsulatedKeySize+aesKeySize {
		return model.Chunk{}, fmt.Errorf("decryption: encapsulated key too short")
	}

	if len(sealed.Nonce) != nonceSize {
		return model.Chunk{}, fmt.Errorf(
			"decryption: invalid nonce size %d, expected %d",
			len(sealed.Nonce),
			nonceSize,
		)
	}

	aesKey, err := s.unwrapAESKey(keyEntry, privKey)
	if err != nil {
		return model.Chunk{}, err
	}

	return s.decryptAndDecompress(sealed, aesKey)
}

func (s *DefaultEncryptionService) unwrapAESKey(
	keyEntry model.KeyEntry,
	privKey []byte,
) ([]byte, error) {
	// Step 1: Parse private key
	privKeyObj, err := parsePrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("decryption: %w", err)
	}

	// Step 2: Extract encapsulated key and wrapped AES key
	encapsulatedKey := keyEntry.EncapsulatedAESKey[:mlkemEncapsulatedKeySize]
	start := mlkemEncapsulatedKeySize
	end := mlkemEncapsulatedKeySize + aesKeySize
	wrappedAESKey := keyEntry.EncapsulatedAESKey[start:end]

	// Step 3: Decapsulate to get shared secret
	sharedSecret, err := privKeyObj.Decapsulate(encapsulatedKey)
	if err != nil {
		return nil, fmt.Errorf("decryption: decapsulating key: %w", err)
	}

	if len(sharedSecret) < aesKeySize {
		return nil, fmt.Errorf("decryption: shared secret too short")
	}

	// Step 4: Unwrap the AES key using XOR with shared secret
	aesKey := make([]byte, aesKeySize)
	for i := 0; i < aesKeySize; i++ {
		aesKey[i] = wrappedAESKey[i] ^ sharedSecret[i]
	}

	return aesKey, nil
}

func (s *DefaultEncryptionService) decryptAndDecompress(
	sealed model.SealedChunk,
	aesKey []byte,
) (model.Chunk, error) {
	// Step 5: Decrypt with AES-256-GCM
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return model.Chunk{}, fmt.Errorf("decryption: creating AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return model.Chunk{}, fmt.Errorf("decryption: creating GCM: %w", err)
	}
	compressed, err := aead.Open(nil, sealed.Nonce, sealed.EncryptedContent, nil)
	if err != nil {
		return model.Chunk{}, fmt.Errorf(
			"decryption: AES-GCM decryption failed: %w",
			err,
		)
	}

	// Step 6: Decompress with zstd
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return model.Chunk{}, fmt.Errorf(
			"decryption: creating zstd decoder: %w",
			err,
		)
	}
	defer decoder.Close()
	content, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return model.Chunk{}, fmt.Errorf(
			"decryption: zstd decompression failed: %w",
			err,
		)
	}

	// Step 7: Verify hash matches
	computedHash := hash.HashBytes(content)
	if computedHash != sealed.ChunkHash {
		return model.Chunk{}, fmt.Errorf(
			"decryption: hash mismatch - content may be corrupted",
		)
	}

	return model.Chunk{
		Hash:    sealed.ChunkHash,
		Size:    len(content),
		Content: content,
	}, nil
}

// GenerateKeyEntry creates a key entry for granting access to encrypted
// content.
//
// This encapsulates the AES key for the specified public key, allowing
// the holder of the corresponding private key to decrypt the content.
//
// pubKey should be the serialized public key, either:
//   - KEM-only: 1568 bytes (ML-KEM-1024 public key)
//   - Full key: 4160 bytes (ML-KEM + ML-DSA public keys concatenated)
func (s *DefaultEncryptionService) GenerateKeyEntry(
	chunkHash hash.Hash,
	pubKey []byte,
	aesKey []byte,
) (model.KeyEntry, error) {
	if len(pubKey) == 0 {
		return model.KeyEntry{}, fmt.Errorf("encryption: public key is required")
	}

	if len(aesKey) != aesKeySize {
		return model.KeyEntry{}, fmt.Errorf(
			"encryption: AES key must be %d bytes, got %d",
			aesKeySize,
			len(aesKey),
		)
	}

	// Parse the public key
	pubKeyObj, err := parsePublicKey(pubKey)
	if err != nil {
		return model.KeyEntry{}, fmt.Errorf("encryption: %w", err)
	}

	// Encapsulate to get shared secret and encapsulated key
	sharedSecret, encapsulatedKey, err := pubKeyObj.Encapsulate()
	if err != nil {
		return model.KeyEntry{}, fmt.Errorf(
			"encryption: encapsulating key: %w",
			err,
		)
	}

	if len(sharedSecret) < aesKeySize {
		return model.KeyEntry{}, fmt.Errorf("encryption: shared secret too short")
	}

	// Wrap the AES key using XOR with shared secret
	wrappedAESKey := make([]byte, aesKeySize)
	for i := 0; i < aesKeySize; i++ {
		wrappedAESKey[i] = aesKey[i] ^ sharedSecret[i]
	}

	// Combine encapsulated key and wrapped AES key
	encapsulatedAESKey := make([]byte, len(encapsulatedKey)+len(wrappedAESKey))
	copy(encapsulatedAESKey, encapsulatedKey)
	copy(encapsulatedAESKey[len(encapsulatedKey):], wrappedAESKey)

	pubKeyHash := hash.HashBytes(pubKey)

	return model.KeyEntry{
		ChunkHash:          chunkHash,
		PubKeyHash:         pubKeyHash,
		EncapsulatedAESKey: encapsulatedAESKey,
	}, nil
}

// parsePublicKey parses a serialized public key into a keys.PublicKey.
// Supports both KEM-only format (1568 bytes) and full format (4160 bytes).
func parsePublicKey(pubKeyBytes []byte) (*keys.PublicKey, error) {
	switch len(pubKeyBytes) {
	case mlkemPublicKeySize:
		// KEM-only format - create a dummy sign key for parsing
		// We'll only use the KEM portion for encryption
		dummySignKey := make([]byte, mldsaPublicKeySize)
		return keys.NewPublicKeyFromBinary(pubKeyBytes, dummySignKey)

	case mlkemPublicKeySize + mldsaPublicKeySize:
		// Full format - split into KEM and Sign portions
		kemBytes := pubKeyBytes[:mlkemPublicKeySize]
		signBytes := pubKeyBytes[mlkemPublicKeySize:]
		return keys.NewPublicKeyFromBinary(kemBytes, signBytes)

	default:
		return nil, fmt.Errorf(
			"invalid public key size %d, expected %d (KEM-only) or %d (full)",
			len(
				pubKeyBytes,
			),
			mlkemPublicKeySize,
			mlkemPublicKeySize+mldsaPublicKeySize,
		)
	}
}

// parsePrivateKey parses a serialized private key into a keys.PrivateKey.
// Supports both KEM-only format (3168 bytes) and full format (8064 bytes).
func parsePrivateKey(privKeyBytes []byte) (*keys.PrivateKey, error) {
	switch len(privKeyBytes) {
	case mlkemPrivateKeySize:
		// KEM-only format - create a dummy sign key for parsing
		dummySignKey := make([]byte, mldsaPrivateKeySize)
		return keys.NewPrivateKeyFromBinary(privKeyBytes, dummySignKey)

	case mlkemPrivateKeySize + mldsaPrivateKeySize:
		// Full format - split into KEM and Sign portions
		kemBytes := privKeyBytes[:mlkemPrivateKeySize]
		signBytes := privKeyBytes[mlkemPrivateKeySize:]
		return keys.NewPrivateKeyFromBinary(kemBytes, signBytes)

	default:
		return nil, fmt.Errorf(
			"invalid private key size %d, expected %d (KEM-only) or %d (full)",
			len(
				privKeyBytes,
			),
			mlkemPrivateKeySize,
			mlkemPrivateKeySize+mldsaPrivateKeySize,
		)
	}
}

// Ensure DefaultEncryptionService implements the EncryptionService interface.
var _ encryption.EncryptionService = (*DefaultEncryptionService)(nil)
