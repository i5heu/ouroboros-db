package cas

import (
	"bytes"
	"fmt"
	"io"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/api"
	chunker "github.com/ipfs/boxo/chunker"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/reedsolomon"
)

func chunkerFunc(payload []byte) ([][]byte, error) {
	reader := bytes.NewReader(payload)
	bz := chunker.NewBuzhash(reader)

	var chunks [][]byte

	for chunkIndex := 0; ; chunkIndex++ {
		buzChunk, err := bz.NextBytes()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, buzChunk)

	}
	return chunks, nil
}

func compressWithZstd(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	_, err = enc.Write(data)
	if err != nil {
		return nil, err
	}
	err = enc.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func splitIntoRSSlicesAndEncrypt(rsDataSlices, rsParitySlices uint8, compressedChunks [][]byte, chunkHashes []hash.Hash, c *crypt.Crypt) ([]api.SealedSlice, error) {
	enc, err := reedsolomon.New(int(rsDataSlices), int(rsParitySlices))
	if err != nil {
		return nil, fmt.Errorf("error creating reed solomon encoder: %w", err)
	}

	var records []api.SealedSlice
	for i, compressedChunk := range compressedChunks {
		// Store original compressed size for reconstruction
		originalSize := uint64(len(compressedChunk))

		// Split compressed chunk into Reed-Solomon slices BEFORE encryption
		slices, err := enc.Split(compressedChunk)
		if err != nil {
			return nil, fmt.Errorf("error splitting compressed chunk: %w", err)
		}

		if len(slices) != int(rsDataSlices+rsParitySlices) {
			return nil, fmt.Errorf("unexpected number of slices: got %d, expected %d", len(slices), rsDataSlices+rsParitySlices)
		}

		// Hash the compressed chunk (before RS encoding) for SealedHash
		compressedHash := hash.HashBytes(compressedChunk)

		// Encrypt each slice individually
		for j, sliceData := range slices {
			// Each slice gets its own encryption with unique key/nonce
			sealedSlice, err := c.Encrypt(sliceData)
			if err != nil {
				return nil, fmt.Errorf("error encrypting slice %d: %w", j, err)
			}

			record := api.SealedSlice{
				ChunkHash:       chunkHashes[i],
				SealedHash:      compressedHash,
				RSDataSlices:    rsDataSlices,
				RSParitySlices:  rsParitySlices,
				RSSliceIndex:    uint8(j),
				Size:            uint64(len(sealedSlice.Ciphertext)),
				OriginalSize:    originalSize,
				EncapsulatedKey: sealedSlice.EncapsulatedKey,
				Nonce:           sealedSlice.Nonce,
				Payload:         sealedSlice.Ciphertext,
			}
			records = append(records, record)
		}
	}

	return records, nil
}

func EncodePayload(payload []byte, rsDataSlices, rsParitySlices uint8, c *crypt.Crypt) ([]api.SealedSlice, []hash.Hash, error) {
	if len(payload) == 0 {
		return nil, nil, nil
	}

	chunks, err := chunkerFunc(payload)
	if err != nil {
		return nil, nil, err
	}

	var chunkHashes []hash.Hash
	for _, chunk := range chunks {
		chunkHashes = append(chunkHashes, hash.HashBytes(chunk))
	}

	var compressedChunks [][]byte
	for _, chunk := range chunks {
		compressedChunk, err := compressWithZstd(chunk)
		if err != nil {
			return nil, nil, err
		}
		compressedChunks = append(compressedChunks, compressedChunk)
	}

	// Split into RS slices and encrypt each slice (not the chunk)
	slices, err := splitIntoRSSlicesAndEncrypt(rsDataSlices, rsParitySlices, compressedChunks, chunkHashes, c)
	if err != nil {
		return nil, nil, err
	}

	return slices, chunkHashes, nil
}
