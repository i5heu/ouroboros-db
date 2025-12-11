package cas

import (
	"bytes"
	"context"
	"fmt"
	"io"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	buzChunker "github.com/ipfs/boxo/chunker"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/reedsolomon"
)

type Chunk struct {
	cas     *CAS
	Hash    hash.Hash // Hash of the content
	Size    int       // Size of the content in bytes
	content []byte    // cached content
}

func NewChunk(cas *CAS, h hash.Hash, size int) Chunk {
	return Chunk{
		cas:  cas,
		Hash: h,
		Size: size,
	}
}

func (c *Chunk) GetContent(cr crypt.Crypt) ([]byte, error) {
	if c.content != nil {
		return c.content, nil
	}

	sealedSlices, err := c.cas.dr.GetSealedSlicesForChunk(c.Hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get sealed slices for chunk: %w", err)
	}

	selectedSlices, err := selectSealedSlicesForReconstruction(sealedSlices)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to select sealed slices for reconstruction: %w",
			err,
		)
	}
	selectedSliceContents := make([][]byte, len(selectedSlices))
	for i, sealedSlice := range selectedSlices {
		sliceContent, err := sealedSlice.Decrypt(cr)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to decrypt sealed slice with hash %s: %w",
				sealedSlice.Hash.String(),
				err,
			)
		}

		if sliceContent == nil {
			return nil, fmt.Errorf(
				"failed to decrypt sealed slice with hash %s",
				sealedSlice.Hash.String(),
			)
		}
		selectedSliceContents[i] = sliceContent
	}

	// Reconstruct the original chunk using Reed-Solomon
	rs, err := reedsolomon.New(
		int(sealedSlices[0].RSDataSlices),
		int(selectedSlices[0].RSParitySlices),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Reed-Solomon encoder: %w", err)
	}

	writer := &bytes.Buffer{}

	err = rs.Join(
		writer,
		selectedSliceContents,
		int(c.Size),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct chunk: %w", err)
	}

	// Decompress the reconstructed chunk with zstd
	content, err := decompressChunkData(writer.Bytes())
	if err != nil {
		return nil, fmt.Errorf("failed to decompress chunk data: %w", err)
	}

	c.content = content
	return c.content, nil
}

func (cas *CAS) storeChunksFromBlob(
	ctx context.Context,
	blobByte []byte,
	c crypt.Crypt,
) ([]Chunk, error) {
	// Check if dr or ki are nil
	if cas.dr == nil {
		return nil, fmt.Errorf("dataRouter is nil")
	}
	if cas.ki == nil {
		return nil, fmt.Errorf("keyIndex is nil")
	}

	// Chunk
	chunkBytes, err := chunker(blobByte)
	if err != nil {
		return nil, fmt.Errorf("failed to chunk blob: %w", err)
	}

	type HashAndSlices struct {
		ChunkHash hash.Hash

		// TODO: We do not really need to keep this in memory
		// but maybe we need it later for something i cannot see now
		SealedSlices []SealedSlice
	}
	hashAndSlices := []HashAndSlices{}

	for _, chunkByte := range chunkBytes {

		chunkHash := hash.HashBytes(chunkByte)

		chunkSealedSlices, err := storeSealedSlicesFromChunk(
			ctx,
			storeSealedSlicesFromChunkOpts{
				CAS:             cas,
				Crypt:           c,
				ClearChunkBytes: chunkByte,
				ChunkHash:       chunkHash,
				RSDataSlices:    4, // TODO make configurable
				RSParitySlices:  3,
			},
		)
		if err != nil {
			return nil, err
		}

		hashAndSlices = append(hashAndSlices, HashAndSlices{
			ChunkHash:    chunkHash,
			SealedSlices: chunkSealedSlices,
		})
	}

	chunks := []Chunk{}
	for _, has := range hashAndSlices {
		chunk := Chunk{
			cas:  cas,
			Hash: has.ChunkHash,
		}

		err = cas.dr.SetChunk(ctx, chunk)
		if err != nil {
			return nil, err
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

func chunker(payload []byte) ([][]byte, error) {
	reader := bytes.NewReader(payload)
	bz := buzChunker.NewBuzhash(reader)

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

func decompressChunkData(compressedData []byte) ([]byte, error) {
	// Create a new zstd decoder
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create zstd decoder: %w", err)
	}
	defer decoder.Close()

	// Decompress the data
	decompressedData, err := decoder.DecodeAll(compressedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decompress data: %w", err)
	}

	return decompressedData, nil
}
