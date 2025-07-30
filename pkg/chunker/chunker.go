package chunker

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-db/pkg/types"
	chunker "github.com/ipfs/boxo/chunker"
	"github.com/klauspost/reedsolomon"
	"github.com/ulikunitz/xz/lzma"
)

func ChunkBytes(data []byte, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8, dataEncryptionKey [32]byte) (chan types.Chunk, chan error) {
	reader := bytes.NewReader(data)
	return ChunkReader(reader, reedSolomonDataChunks, reedSolomonParityChunks, dataEncryptionKey)
}

// ChunkReader will chunk the data from the reader and return the chunks through a channel
// The chunk size is minimum/maximum = 128K/512K
func ChunkReader(reader io.Reader, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8, dataEncryptionKey [32]byte) (chan types.Chunk, chan error) {
	resultChan := make(chan types.Chunk, 20)
	errorChan := make(chan error)
	bz := chunker.NewBuzhash(reader)

	c, err := aes.NewCipher(dataEncryptionKey[:])
	if err != nil {
		errorChan <- fmt.Errorf("error creating cipher: %w", err)
		return nil, errorChan
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
	}

	go func() {
		defer close(resultChan)
		defer close(errorChan)

		for chunkIndex := 0; ; chunkIndex++ {
			buzChunk, err := bz.NextBytes()
			if err == io.EOF {
				break
			}
			if err != nil {
				errorChan <- fmt.Errorf("error reading chunk: %w", err)
				return
			}

			// Create an encoder with 7 data and 3 parity slices.
			enc, err := reedsolomon.New(int(reedSolomonDataChunks), int(reedSolomonParityChunks))
			if err != nil {
				errorChan <- fmt.Errorf("error creating reed solomon encoder: %w", err)
				return
			}

			compressedBuzChunk, err := compressWithLzma(buzChunk)
			if err != nil {
				errorChan <- fmt.Errorf("error compressing chunk: %w", err)
				return
			}

			shards, err := enc.Split(compressedBuzChunk)
			if err != nil {
				errorChan <- fmt.Errorf("error splitting chunk: %w", err)
				return
			}

			err = enc.Encode(shards)
			if err != nil {
				errorChan <- fmt.Errorf("error encoding chunk: %w", err)
				return
			}

			buzChunkHash := sha512.Sum512(buzChunk)

			eccHashes := make([]types.Hash, reedSolomonDataChunks+reedSolomonParityChunks)
			for i := range eccHashes {
				eccHashes[i] = types.Hash(sha512.Sum512(shards[i]))
			}

			var chunkShards types.ECCCollection

			for i := range shards {

				eccChunkHash := sha512.Sum512(shards[i])

				// initialization vector (iv)
				iv := eccChunkHash[:gcm.NonceSize()]

				// encrypt
				encryptedShard := gcm.Seal(nil, iv, []byte(shards[i]), nil)

				chunkData := types.ECChunk{
					ECCMeta: types.ECCMeta{
						ECChunkHash:    eccChunkHash,
						OtherECCHashes: filterOutCurrentECCHash(eccHashes, uint8(i)),

						BuzChunkHash:   buzChunkHash,
						BuzChunkNumber: uint32(chunkIndex),

						ECCNumber:           reedSolomonDataChunks,
						ECC_k:               reedSolomonParityChunks,
						ECC_n:               uint8(i),
						EncryptedDataLength: uint32(len(encryptedShard)),
					},
					EncryptedData: encryptedShard,
				}

				chunkShards = append(chunkShards, chunkData)
			}

			result := types.Chunk{
				ChunkMeta: types.ChunkMeta{
					Hash:       buzChunkHash,
					DataLength: uint32(len(buzChunk)),
				},
				Data: chunkShards,
			}

			resultChan <- result
		}
	}()

	return resultChan, errorChan
}

func filterOutCurrentECCHash(eccHashes []types.Hash, currentECCNumber uint8) []types.Hash {
	filteredHashes := make([]types.Hash, 0, len(eccHashes)-1)
	for i := range eccHashes {
		if uint8(i) != currentECCNumber {
			filteredHashes = append(filteredHashes, eccHashes[i])
		}
	}
	return filteredHashes
}

func compressWithLzma(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w, err := lzma.NewWriter(&buf)
	if err != nil {
		return nil, err
	}
	_, err = w.Write(data)
	if err != nil {
		return nil, err
	}

	err = w.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
