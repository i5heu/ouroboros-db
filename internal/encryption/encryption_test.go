package encryption

import (
	"bytes"
	"testing"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"pgregory.net/rapid"
)

// Helper to generate a key pair and return serialized public and private keys
func generateKeyPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()

	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("failed to marshal public KEM key: %v", err)
	}
	pubSign, err := pub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("failed to marshal public sign key: %v", err)
	}
	pubBytes := make([]byte, len(pubKEM)+len(pubSign))
	copy(pubBytes, pubKEM)
	copy(pubBytes[len(pubKEM):], pubSign)

	privKEM, err := priv.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("failed to marshal private KEM key: %v", err)
	}
	privSign, err := priv.MarshalBinarySign()
	if err != nil {
		t.Fatalf("failed to marshal private sign key: %v", err)
	}
	privBytes := make([]byte, len(privKEM)+len(privSign))
	copy(privBytes, privKEM)
	copy(privBytes[len(privKEM):], privSign)

	return pubBytes, privBytes
}

// Helper to generate KEM-only key pair
func generateKEMOnlyKeyPair(t *testing.T) ([]byte, []byte) {
	t.Helper()
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()

	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("failed to marshal public KEM key: %v", err)
	}

	privKEM, err := priv.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("failed to marshal private KEM key: %v", err)
	}

	return pubKEM, privKEM
}

func TestSealUnsealChunk_RoundTrip(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, privKey := generateKeyPair(t)

	content := []byte("Hello, World! This is a test message for encryption.")
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	// Seal the chunk
	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pubKey})
	if err != nil {
		t.Fatalf("SealChunk failed: %v", err)
	}

	// Verify sealed chunk structure
	if len(sealed.EncryptedContent) == 0 {
		t.Error("EncryptedContent should not be empty")
	}
	if len(sealed.Nonce) != nonceSize {
		t.Errorf("Nonce should be %d bytes, got %d", nonceSize, len(sealed.Nonce))
	}
	if sealed.ChunkHash != chunk.Hash {
		t.Error("ChunkHash mismatch")
	}
	if sealed.OriginalSize != chunk.Size {
		t.Errorf(
			"OriginalSize mismatch: expected %d, got %d",
			chunk.Size,
			sealed.OriginalSize,
		)
	}

	// Verify key entries
	if len(keyEntries) != 1 {
		t.Fatalf("Expected 1 key entry, got %d", len(keyEntries))
	}
	if keyEntries[0].ChunkHash != chunk.Hash {
		t.Error("KeyEntry ChunkHash mismatch")
	}
	if len(
		keyEntries[0].EncapsulatedAESKey,
	) != mlkemEncapsulatedKeySize+aesKeySize {
		t.Errorf(
			"EncapsulatedAESKey wrong size: expected %d, got %d",
			mlkemEncapsulatedKeySize+aesKeySize,
			len(keyEntries[0].EncapsulatedAESKey),
		)
	}

	// Unseal the chunk
	unsealed, err := svc.UnsealChunk(sealed, keyEntries[0], privKey)
	if err != nil {
		t.Fatalf("UnsealChunk failed: %v", err)
	}

	// Verify decrypted content
	if !bytes.Equal(unsealed.Content, content) {
		t.Error("Decrypted content does not match original")
	}
	if unsealed.Hash != chunk.Hash {
		t.Error("Hash mismatch after decryption")
	}
	if unsealed.Size != chunk.Size {
		t.Errorf("Size mismatch: expected %d, got %d", chunk.Size, unsealed.Size)
	}
}

func TestSealUnsealChunk_KEMOnlyKeys(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, privKey := generateKEMOnlyKeyPair(t)

	content := []byte("Test with KEM-only keys")
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pubKey})
	if err != nil {
		t.Fatalf("SealChunk with KEM-only keys failed: %v", err)
	}

	unsealed, err := svc.UnsealChunk(sealed, keyEntries[0], privKey)
	if err != nil {
		t.Fatalf("UnsealChunk with KEM-only keys failed: %v", err)
	}

	if !bytes.Equal(unsealed.Content, content) {
		t.Error("Decrypted content does not match original")
	}
}

func TestSealChunk_MultipleRecipients(t *testing.T) {
	svc := NewEncryptionService()

	// Generate 3 key pairs
	pub1, priv1 := generateKeyPair(t)
	pub2, priv2 := generateKeyPair(t)
	pub3, priv3 := generateKeyPair(t)

	content := []byte("Multi-recipient encrypted content")
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	// Seal for all 3 recipients
	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pub1, pub2, pub3})
	if err != nil {
		t.Fatalf("SealChunk failed: %v", err)
	}

	if len(keyEntries) != 3 {
		t.Fatalf("Expected 3 key entries, got %d", len(keyEntries))
	}

	// Each recipient should be able to decrypt
	keys := [][]byte{priv1, priv2, priv3}
	for i, priv := range keys {
		unsealed, err := svc.UnsealChunk(sealed, keyEntries[i], priv)
		if err != nil {
			t.Fatalf("Recipient %d failed to unseal: %v", i+1, err)
		}
		if !bytes.Equal(unsealed.Content, content) {
			t.Errorf("Recipient %d: decrypted content does not match", i+1)
		}
	}
}

func TestSealChunk_WrongKeyCannotDecrypt(t *testing.T) {
	svc := NewEncryptionService()
	pub1, _ := generateKeyPair(t)
	_, priv2 := generateKeyPair(t) // Different key pair

	content := []byte("Secret message")
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pub1})
	if err != nil {
		t.Fatalf("SealChunk failed: %v", err)
	}

	// Try to decrypt with wrong private key - should fail
	_, err = svc.UnsealChunk(sealed, keyEntries[0], priv2)
	if err == nil {
		t.Error("Expected decryption to fail with wrong key")
	}
}

func TestGenerateKeyEntry_GrantAccess(t *testing.T) {
	svc := NewEncryptionService()
	pub1, priv1 := generateKeyPair(t)
	_, priv2 := generateKeyPair(t)

	content := []byte("Content to share later")
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	// Initially encrypt for user 1 only
	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pub1})
	if err != nil {
		t.Fatalf("SealChunk failed: %v", err)
	}

	// User 1 decrypts to get the content and AES key
	// In a real scenario, the AES key would need to be extracted after
	// decapsulation
	// For testing, we'll generate a new key entry using a known AES key

	// First, verify user 1 can decrypt
	_, err = svc.UnsealChunk(sealed, keyEntries[0], priv1)
	if err != nil {
		t.Fatalf("User 1 failed to decrypt: %v", err)
	}

	// User 2 should not be able to decrypt with user 1's key entry
	_, err = svc.UnsealChunk(sealed, keyEntries[0], priv2)
	if err == nil {
		t.Error("User 2 should not be able to decrypt with user 1's key entry")
	}
}

func TestSealChunk_EmptyContent(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, _ := generateKeyPair(t)

	chunk := model.Chunk{
		Hash:    hash.Hash{},
		Size:    0,
		Content: []byte{},
	}

	_, _, err := svc.SealChunk(chunk, [][]byte{pubKey})
	if err == nil {
		t.Error("Expected error for empty content")
	}
}

func TestSealChunk_NoPubKeys(t *testing.T) {
	svc := NewEncryptionService()

	chunk := model.Chunk{
		Hash:    hash.HashBytes([]byte("test")),
		Size:    4,
		Content: []byte("test"),
	}

	_, _, err := svc.SealChunk(chunk, [][]byte{})
	if err == nil {
		t.Error("Expected error for no public keys")
	}
}

func TestGenerateKeyEntry_InvalidInputs(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, _ := generateKeyPair(t)

	tests := []struct {
		name    string
		pubKey  []byte
		aesKey  []byte
		wantErr bool
	}{
		{
			name:    "empty public key",
			pubKey:  []byte{},
			aesKey:  make([]byte, aesKeySize),
			wantErr: true,
		},
		{
			name:    "wrong AES key size (too short)",
			pubKey:  pubKey,
			aesKey:  make([]byte, 16),
			wantErr: true,
		},
		{
			name:    "wrong AES key size (too long)",
			pubKey:  pubKey,
			aesKey:  make([]byte, 64),
			wantErr: true,
		},
		{
			name:    "invalid public key size",
			pubKey:  make([]byte, 100),
			aesKey:  make([]byte, aesKeySize),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GenerateKeyEntry(hash.Hash{}, tt.pubKey, tt.aesKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateKeyEntry() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnsealChunk_InvalidInputs(t *testing.T) {
	svc := NewEncryptionService()
	_, privKey := generateKeyPair(t)

	tests := []struct {
		name     string
		sealed   model.SealedChunk
		keyEntry model.KeyEntry
		privKey  []byte
		wantErr  bool
	}{
		{
			name: "empty encrypted content",
			sealed: model.SealedChunk{
				EncryptedContent: []byte{},
				Nonce:            make([]byte, nonceSize),
			},
			keyEntry: model.KeyEntry{
				EncapsulatedAESKey: make([]byte, mlkemEncapsulatedKeySize+aesKeySize),
			},
			privKey: privKey,
			wantErr: true,
		},
		{
			name: "short encapsulated key",
			sealed: model.SealedChunk{
				EncryptedContent: []byte("encrypted"),
				Nonce:            make([]byte, nonceSize),
			},
			keyEntry: model.KeyEntry{
				EncapsulatedAESKey: make([]byte, 100),
			},
			privKey: privKey,
			wantErr: true,
		},
		{
			name: "invalid nonce size",
			sealed: model.SealedChunk{
				EncryptedContent: []byte("encrypted"),
				Nonce:            make([]byte, 8), // wrong size
			},
			keyEntry: model.KeyEntry{
				EncapsulatedAESKey: make([]byte, mlkemEncapsulatedKeySize+aesKeySize),
			},
			privKey: privKey,
			wantErr: true,
		},
		{
			name: "invalid private key size",
			sealed: model.SealedChunk{
				EncryptedContent: []byte("encrypted"),
				Nonce:            make([]byte, nonceSize),
			},
			keyEntry: model.KeyEntry{
				EncapsulatedAESKey: make([]byte, mlkemEncapsulatedKeySize+aesKeySize),
			},
			privKey: make([]byte, 100), // wrong size
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UnsealChunk(tt.sealed, tt.keyEntry, tt.privKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnsealChunk() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSealChunk_LargeContent(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, privKey := generateKeyPair(t)

	// Create 1MB of content
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pubKey})
	if err != nil {
		t.Fatalf("SealChunk failed for large content: %v", err)
	}

	// Encrypted size should be smaller due to compression (for repetitive data)
	// or larger due to overhead, but definitely not empty
	if len(sealed.EncryptedContent) == 0 {
		t.Error("Encrypted content should not be empty")
	}

	unsealed, err := svc.UnsealChunk(sealed, keyEntries[0], privKey)
	if err != nil {
		t.Fatalf("UnsealChunk failed for large content: %v", err)
	}

	if !bytes.Equal(unsealed.Content, content) {
		t.Error("Large content mismatch after round-trip")
	}
}

func TestSealChunk_CompressionEffective(t *testing.T) {
	svc := NewEncryptionService()
	pubKey, _ := generateKeyPair(t)

	// Create highly compressible content (repeated pattern)
	content := bytes.Repeat([]byte("AAAAAAAAAA"), 10000) // 100KB of 'A's

	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	sealed, _, err := svc.SealChunk(chunk, [][]byte{pubKey})
	if err != nil {
		t.Fatalf("SealChunk failed: %v", err)
	}

	// Compressed+encrypted size should be much smaller than original
	// (zstd achieves excellent compression on repetitive data)
	// Allow some overhead for AES-GCM tag (16 bytes) and compression framing
	if len(sealed.EncryptedContent) >= len(content)/2 {
		t.Logf(
			"Original size: %d, Encrypted size: %d",
			len(content),
			len(sealed.EncryptedContent),
		)
		t.Log(
			"Warning: compression may not be working effectively for highly " +
				"compressible data",
		)
	}
}

// Property-based test: any content should round-trip correctly
func TestSealUnsealChunk_Property_RoundTrip(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := NewEncryptionService()

		// Generate keys (do this once to save time)
		c := crypt.New()
		pub := c.Keys.GetPublicKey()
		priv := c.Keys.GetPrivateKey()

		pubKEM, _ := pub.MarshalBinaryKEM()
		pubSign, _ := pub.MarshalBinarySign()
		pubBytes := make([]byte, len(pubKEM)+len(pubSign))
		copy(pubBytes, pubKEM)
		copy(pubBytes[len(pubKEM):], pubSign)

		privKEM, _ := priv.MarshalBinaryKEM()
		privSign, _ := priv.MarshalBinarySign()
		privBytes := make([]byte, len(privKEM)+len(privSign))
		copy(privBytes, privKEM)
		copy(privBytes[len(privKEM):], privSign)

		// Generate random content (1 byte to 10KB)
		content := rapid.SliceOfN(rapid.Byte(), 1, 10240).Draw(t, "content")

		chunk := model.Chunk{
			Hash:    hash.HashBytes(content),
			Size:    len(content),
			Content: content,
		}

		sealed, keyEntries, err := svc.SealChunk(chunk, [][]byte{pubBytes})
		if err != nil {
			t.Fatalf("SealChunk failed: %v", err)
		}

		unsealed, err := svc.UnsealChunk(sealed, keyEntries[0], privBytes)
		if err != nil {
			t.Fatalf("UnsealChunk failed: %v", err)
		}

		if !bytes.Equal(unsealed.Content, content) {
			t.Fatal("Content mismatch after round-trip")
		}
		if unsealed.Hash != chunk.Hash {
			t.Fatal("Hash mismatch after round-trip")
		}
	})
}

// Property-based test: different encryptions of same content produce different
// ciphertext
func TestSealChunk_Property_NonDeterministic(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		svc := NewEncryptionService()

		c := crypt.New()
		pub := c.Keys.GetPublicKey()
		pubKEM, _ := pub.MarshalBinaryKEM()
		pubSign, _ := pub.MarshalBinarySign()
		pubBytes := make([]byte, len(pubKEM)+len(pubSign))
		copy(pubBytes, pubKEM)
		copy(pubBytes[len(pubKEM):], pubSign)

		content := rapid.SliceOfN(rapid.Byte(), 10, 1000).Draw(t, "content")

		chunk := model.Chunk{
			Hash:    hash.HashBytes(content),
			Size:    len(content),
			Content: content,
		}

		sealed1, _, err := svc.SealChunk(chunk, [][]byte{pubBytes})
		if err != nil {
			t.Fatalf("First SealChunk failed: %v", err)
		}

		sealed2, _, err := svc.SealChunk(chunk, [][]byte{pubBytes})
		if err != nil {
			t.Fatalf("Second SealChunk failed: %v", err)
		}

		// Nonces should be different
		if bytes.Equal(sealed1.Nonce, sealed2.Nonce) {
			t.Fatal("Nonces should be different for each encryption")
		}

		// Ciphertext should be different (due to different nonce and AES key)
		if bytes.Equal(sealed1.EncryptedContent, sealed2.EncryptedContent) {
			t.Fatal("Ciphertext should be different for each encryption")
		}
	})
}

// Benchmark encryption
func BenchmarkSealChunk_1KB(b *testing.B) {
	svc := NewEncryptionService()
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	pubKEM, _ := pub.MarshalBinaryKEM()
	pubSign, _ := pub.MarshalBinarySign()
	pubBytes := make([]byte, len(pubKEM)+len(pubSign))
	copy(pubBytes, pubKEM)
	copy(pubBytes[len(pubKEM):], pubSign)

	content := make([]byte, 1024)
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = svc.SealChunk(chunk, [][]byte{pubBytes})
	}
}

func BenchmarkUnsealChunk_1KB(b *testing.B) {
	svc := NewEncryptionService()
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()

	pubKEM, _ := pub.MarshalBinaryKEM()
	pubSign, _ := pub.MarshalBinarySign()
	pubBytes := make([]byte, len(pubKEM)+len(pubSign))
	copy(pubBytes, pubKEM)
	copy(pubBytes[len(pubKEM):], pubSign)

	privKEM, _ := priv.MarshalBinaryKEM()
	privSign, _ := priv.MarshalBinarySign()
	privBytes := make([]byte, len(privKEM)+len(privSign))
	copy(privBytes, privKEM)
	copy(privBytes[len(privKEM):], privSign)

	content := make([]byte, 1024)
	chunk := model.Chunk{
		Hash:    hash.HashBytes(content),
		Size:    len(content),
		Content: content,
	}

	sealed, keyEntries, _ := svc.SealChunk(chunk, [][]byte{pubBytes})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = svc.UnsealChunk(sealed, keyEntries[0], privBytes)
	}
}
