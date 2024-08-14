package storage

import (
	"bytes"

	"crypto/sha256"
	"crypto/sha512"

	"github.com/i5heu/ouroboros-db/pkg/buzhashChunker"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/ulikunitz/xz/lzma"
	"golang.org/x/crypto/chacha20poly1305"
)

type ChunkData struct {
	Hash types.Hash
	Data []byte
}

func (s *Storage) StoreDataPipeline(data []byte) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	room := s.wp.CreateRoom(100)
	room.AsyncCollector()

	pass := "HelloWorld"

	key := sha256.Sum256([]byte(pass))
	aead, err := chacha20poly1305.NewX(key[:32])
	if err != nil {
		return nil, nil, err
	}

	chunkResults, _ := buzhashChunker.ChunkBytes(data)

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

			// encrypt
			nonce := make([]byte, chacha20poly1305.NonceSizeX)
			cipherdata := aead.Seal(nil, nonce, []byte(compressedChunk), nil)

			aead.Open(nil, nonce, cipherdata, nil)

			// erasure code
			// todo

			return ChunkData{
				Hash: types.Hash(hashOfChunk[:]),
				Data: cipherdata,
			}
		})
	}

	chunkData, err := room.GetAsyncResults()
	if err != nil {
		return nil, nil, err
	}

	var chunkResult []ChunkData

	for _, cd := range chunkData {
		chunkResult = append(chunkResult, cd.(ChunkData))
	}

	chunkMetas := make(types.ChunkMetaCollection, len(chunkData))
	chunks := make(types.ChunkCollection, len(chunkData))
	return chunks, chunkMetas, nil
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
