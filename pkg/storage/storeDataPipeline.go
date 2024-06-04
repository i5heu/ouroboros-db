package storage

import (
	"bytes"
	"crypto/sha512"
	"encoding/hex"
	"fmt"

	"github.com/cloudflare/circl/pke/kyber/kyber1024"
	"github.com/i5heu/ouroboros-db/pkg/buzhashChunker"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/ulikunitz/xz/lzma"
)

type ChunkData struct {
	Hash types.Hash
	Data []byte
}

func (s *Storage) StoreDataPipeline(data []byte) (types.ChunkCollection, types.ChunkMetaCollection, error) {
	room := s.wp.CreateRoom(100)
	room.AsyncCollector()

	pub, _, err := kyber1024.GenerateKey(nil)
	if err != nil {
		return nil, nil, err
	}

	chunkResults, _ := buzhashChunker.ChunkBytes(data)

	fmt.Println("start 1")
	for chunkTmp := range chunkResults {
		chunk := chunkTmp
		room.NewTaskWaitForFreeSlot(func() interface{} {
			fmt.Println("start 2")
			// sha512
			hashOfChunk := sha512.Sum512(chunk.Data)

			// compress use xz
			compressedChunk, err := compressWithLzma(chunk.Data)
			if err != nil {
				return err
			}

			// encrypt
			var encryptedChunk []byte
			pub.EncryptTo(encryptedChunk, compressedChunk, nil)

			// erasure code
			fmt.Println("start 3")

			return ChunkData{
				Hash: types.Hash(hashOfChunk[:]),
				Data: compressedChunk,
			}
		})
	}

	fmt.Println("start 4")

	chunkData, err := room.GetAsyncResults()
	if err != nil {
		return nil, nil, err
	}

	var chunkResult []ChunkData

	for _, cd := range chunkData {
		chunkResult = append(chunkResult, cd.(ChunkData))
	}

	fmt.Println("start 5")

	fmt.Println(hex.EncodeToString(chunkResult[0].Data))

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
