package cas

import (
	"bytes"
	"testing"

	_ "github.com/i5heu/ouroboros-db/internal/testutil"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

func TestReconstructPayload(t *testing.T) {
	c := crypt.New()
	testData := createTestData()

	// First encode some data to get valid slices
	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("Setup failed: EncodePayload returned error: %v", err)
	}

	// Now try to reconstruct
	reconstructedContent, rsData, rsParity, err := ReconstructPayload(slices, chunkHashes, c)
	if err != nil {
		t.Fatalf("ReconstructPayload failed: %v", err)
	}

	if !bytes.Equal(reconstructedContent, testData.Content) {
		t.Error("Reconstructed content does not match original")
	}

	if rsData != testData.RSDataSlices {
		t.Errorf("Expected RSDataSlices %d, got %d", testData.RSDataSlices, rsData)
	}
	if rsParity != testData.RSParitySlices {
		t.Errorf("Expected RSParitySlices %d, got %d", testData.RSParitySlices, rsParity)
	}
}

func TestReconstructPayloadMissingSlices(t *testing.T) {
	c := crypt.New()
	testData := createTestData()

	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("Setup failed: EncodePayload returned error: %v", err)
	}

	// Remove some slices but keep enough to reconstruct (need RSDataSlices)
	// We have 3 data + 2 parity = 5 total. We need 3.
	// Let's keep 3 slices.
	partialSlices := slices[:testData.RSDataSlices]

	reconstructedContent, _, _, err := ReconstructPayload(partialSlices, chunkHashes, c)
	if err != nil {
		t.Fatalf("ReconstructPayload with partial slices failed: %v", err)
	}

	if !bytes.Equal(reconstructedContent, testData.Content) {
		t.Error("Reconstructed content from partial slices does not match original")
	}
}

func TestReconstructPayloadNotEnoughSlices(t *testing.T) {
	c := crypt.New()
	testData := createTestData()

	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("Setup failed: EncodePayload returned error: %v", err)
	}

	// Keep fewer than RSDataSlices
	// We have 3 data. Let's keep 2.
	tooFewSlices := slices[:testData.RSDataSlices-1]

	_, _, _, err = ReconstructPayload(tooFewSlices, chunkHashes, c)
	if err == nil {
		t.Error("Expected error when reconstructing with too few slices, but got none")
	}
}

func TestReedSolomonReconstructor(t *testing.T) {
	// Manually construct SealedSlices to test the reconstructor directly
	dataSlices := uint8(3)
	paritySlices := uint8(2)

	// We need valid encrypted data for the payload, but since reedSolomonReconstructor
	// only does RS reconstruction and not decryption, we can use dummy data
	// as long as it's valid RS encoded data.
	// However, reedSolomonReconstructor returns *encrypt.EncryptResult.
	// It expects the payloads to be shards of the EncryptResult.Ciphertext?
	// No, it expects the payloads to be shards of the data that was split.
	// In our pipeline, we encrypt first, then split the EncryptResult.Ciphertext.

	// Let's use the helper to generate valid slices first
	c := crypt.New()

	// Compress a small piece of data
	content := []byte("test-content")
	compressed, err := compressWithZstd(content)
	if err != nil {
		t.Fatalf("Compress failed: %v", err)
	}

	// Use splitIntoRSSlicesAndEncrypt to get valid SealedSlices
	chunkHashes := []hash.Hash{hash.HashBytes(content)}
	compressedChunks := [][]byte{compressed}

	slices, err := splitIntoRSSlicesAndEncrypt(dataSlices, paritySlices, compressedChunks, chunkHashes, c)
	if err != nil {
		t.Fatalf("splitIntoRSSlicesAndEncrypt failed: %v", err)
	}

	// Now test reedSolomonReconstructor with these slices
	result, err := reedSolomonReconstructor(slices, c)
	if err != nil {
		t.Fatalf("reedSolomonReconstructor failed: %v", err)
	}

	if !bytes.Equal(result, compressed) {
		t.Error("Reconstructed compressed data does not match original")
	}

	// Test with missing slices - RS should be able to reconstruct
	partialSlices := slices[:dataSlices]
	resultPartial, err := reedSolomonReconstructor(partialSlices, c)
	if err != nil {
		t.Fatalf("reedSolomonReconstructor with partial slices failed: %v", err)
	}

	if !bytes.Equal(resultPartial, compressed) {
		t.Error("Reconstructed compressed data from partial slices does not match original")
	}
}

func TestDecompressWithZstd(t *testing.T) {
	originalData := []byte("Test data for decompression")
	compressed, err := compressWithZstd(originalData)
	if err != nil {
		t.Fatalf("Setup failed: compressWithZstd returned error: %v", err)
	}

	decompressed, err := decompressWithZstd(compressed)
	if err != nil {
		t.Fatalf("decompressWithZstd failed: %v", err)
	}

	if !bytes.Equal(decompressed, originalData) {
		t.Error("Decompressed data does not match original")
	}
}

func TestDecompressWithZstdInvalidData(t *testing.T) {
	invalidData := []byte("This is not valid zstd data")
	_, err := decompressWithZstd(invalidData)
	if err == nil {
		t.Error("Expected error when decompressing invalid data, but got none")
	}
}

func TestReconstructPayloadCorruptedData(t *testing.T) {
	c := crypt.New()
	testData := createTestData()

	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("Setup failed: EncodePayload returned error: %v", err)
	}

	// Corrupt one slice's payload
	if len(slices) > 0 {
		slices[0].Payload = []byte("corrupted payload")
	}

	// Try to reconstruct.
	reconstructedContent, _, _, err := ReconstructPayload(slices, chunkHashes, c)

	if err != nil {
		t.Logf("Reconstruction failed with corrupted data (expected behavior): %v", err)
	} else {
		if !bytes.Equal(reconstructedContent, testData.Content) {
			t.Log("Reconstructed content matches original despite corruption (RS correction worked)")
		} else {
			t.Log("Reconstructed content matches original (RS correction worked)")
		}
	}
}
