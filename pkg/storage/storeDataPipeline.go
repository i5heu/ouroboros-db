package storage

import (
	"bytes"

	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"

	"github.com/i5heu/ouroboros-db/pkg/chunker"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/ulikunitz/xz/lzma"
)

func (s *Storage) StoreDataPipeline(data []byte, reedSolomonDataChunks uint8, reedSolomonParityChunks uint8) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	room := s.wp.CreateRoom(100)
	room.AsyncCollector()

	c, err := aes.NewCipher(s.dataEncryptionKey[:])
	if err != nil {
		return nil, nil, err
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, nil, err
	}

	chunkResults, _ := chunker.ChunkBytes(data, reedSolomonDataChunks, reedSolomonParityChunks)

	for chunkTmp := range chunkResults {
		chunk := chunkTmp
		room.NewTaskWaitForFreeSlot(func() interface{} {
			// sha512
			hashOfChunk := sha512.Sum512(chunk.Data)

			// compress use xz
			compressedChunk, err := compressWithLzma(chunk.Data)
			if err != nil {
				return err
			}

			// initialization vector (iv)
			iv := hashOfChunk[:gcm.NonceSize()]

			// encrypt
			cipherdata := gcm.Seal(nil, iv, []byte(compressedChunk), nil)

			// // decrypt
			// _, err = gcm.Open(nil, iv, cipherdata, nil)
			// if err != nil {
			// 	return err
			// }

			return chunker.ECChunk{
				Hash:                    types.Hash(hashOfChunk[:]),
				ReedSolomonDataChunks:   chunk.ECCNumber,
				ReedSolomonParityChunks: chunk.ECC_k,
				ReedSolomonParityShard:  chunk.ECC_n,
				Data:                    cipherdata,
			}
		})
	}

	chunkData, err := room.GetAsyncResults()
	if err != nil {
		return nil, nil, err
	}

	var chunks types.ChunkCollection
	chunkMetaCollection := make(types.ChunkMetaCollection, len(chunkData))

	for i, chunk := range chunkData {
		data := chunk.(chunker.ECChunk).Data
		hash := chunk.(chunker.ECChunk).Hash

		chunkMeta := types.ChunkMeta{
			Hash:                    hash,
			ReedSolomonDataChunks:   chunk.(chunker.ECChunk).ReedSolomonDataChunks,
			ReedSolomonParityChunks: chunk.(chunker.ECChunk).ReedSolomonParityChunks,
			ReedSolomonParityShard:  chunk.(chunker.ECChunk).ReedSolomonParityShard,
			DataLength:              uint32(len(data)),
		}
		chunkMetaCollection[i] = chunkMeta

		chunks = append(chunks, types.Chunk{
			ChunkMeta: chunkMeta,
			Data:      data,
		})
	}

	return chunks, chunkMetaCollection, nil
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

func decompressWithLzma(data []byte) ([]byte, error) {
	r, err := lzma.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
