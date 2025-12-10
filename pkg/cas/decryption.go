package cas

import (
	"bytes"
	"fmt"
	"io"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/api"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/reedsolomon"
)

func reedSolomonReconstructor(slices []api.SealedSlice, c *crypt.Crypt) ([]byte, error) {
	if len(slices) == 0 {
		return nil, fmt.Errorf("no slices provided for reconstruction")
	}

	// All slices should have the same Reed-Solomon configuration
	firstSlice := slices[0]
	dataSlices := int(firstSlice.RSDataSlices)
	paritySlices := int(firstSlice.RSParitySlices)
	totalSlices := dataSlices + paritySlices

	if len(slices) > totalSlices {
		return nil, fmt.Errorf("too many slices: got %d, expected at most %d", len(slices), totalSlices)
	}

	// Create Reed-Solomon decoder
	enc, err := reedsolomon.New(dataSlices, paritySlices)
	if err != nil {
		return nil, fmt.Errorf("failed to create Reed-Solomon decoder: %w", err)
	}

	// Prepare slice matrix - decrypt each slice first
	stripeSlices := make([][]byte, totalSlices)

	// Decrypt each slice individually and fill slice matrix
	for _, slice := range slices {
		if int(slice.RSSliceIndex) >= totalSlices {
			return nil, fmt.Errorf("invalid Reed-Solomon index: %d (max %d)", slice.RSSliceIndex, totalSlices-1)
		}

		// Decrypt this slice using its own key/nonce
		encryptedSlice := &encrypt.EncryptResult{
			Ciphertext:      slice.Payload,
			EncapsulatedKey: slice.EncapsulatedKey,
			Nonce:           slice.Nonce,
		}

		decryptedSlice, err := c.Decrypt(encryptedSlice)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt slice %d: %w", slice.RSSliceIndex, err)
		}

		stripeSlices[slice.RSSliceIndex] = decryptedSlice
	}

	// Try to reconstruct missing slices
	err = enc.Reconstruct(stripeSlices)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct Reed-Solomon slices: %w", err)
	}

	// Calculate the total expected data size from the original size stored in slices
	originalSize := int(slices[0].OriginalSize)

	// Use the Reed-Solomon Join method to properly reconstruct the compressed data
	var reconstructed bytes.Buffer
	err = enc.Join(&reconstructed, stripeSlices, originalSize)
	if err != nil {
		return nil, fmt.Errorf("failed to join Reed-Solomon slices: %w", err)
	}

	return reconstructed.Bytes(), nil
}

func decompressWithZstd(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	dec, err := zstd.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create Zstd reader: %w", err)
	}
	defer dec.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, dec)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress Zstd data: %w", err)
	}

	return buf.Bytes(), nil
}

func ReconstructPayload(slices []api.SealedSlice, hashOrder []hash.Hash, c *crypt.Crypt) ([]byte, uint8, uint8, error) {
	if len(slices) == 0 {
		return nil, 0, 0, nil
	}

	if len(hashOrder) == 0 {
		return nil, 0, 0, fmt.Errorf("hash order missing for payload reconstruction")
	}

	sliceGroups := make(map[hash.Hash][]api.SealedSlice)
	for _, slice := range slices {
		sliceGroups[slice.ChunkHash] = append(sliceGroups[slice.ChunkHash], slice)
	}

	var payload bytes.Buffer
	var rsData, rsParity uint8

	for idx, chunkHash := range hashOrder {
		stripeSlices, ok := sliceGroups[chunkHash]
		if !ok {
			return nil, 0, 0, fmt.Errorf("missing slice group for hash %x", chunkHash)
		}

		// Reconstruct compressed chunk from decrypted slices
		compressedChunk, err := reedSolomonReconstructor(stripeSlices, c)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to reconstruct Reed-Solomon chunk: %w", err)
		}

		// Decompress the reconstructed chunk
		decompressedChunk, err := decompressWithZstd(compressedChunk)
		if err != nil {
			return nil, 0, 0, fmt.Errorf("failed to decompress chunk: %w", err)
		}

		// Verify chunk hash matches expected plaintext hash
		actualHash := hash.HashBytes(decompressedChunk)
		if actualHash != chunkHash {
			return nil, 0, 0, fmt.Errorf("chunk %d hash mismatch: expected %x, got %x", idx, chunkHash, actualHash)
		}

		payload.Write(decompressedChunk)

		if rsData == 0 && len(stripeSlices) > 0 {
			rsData = stripeSlices[0].RSDataSlices
			rsParity = stripeSlices[0].RSParitySlices
		}
	}

	return payload.Bytes(), rsData, rsParity, nil
}
