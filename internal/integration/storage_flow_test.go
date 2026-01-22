package integration

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/internal/blockstore"
	"github.com/i5heu/ouroboros-db/internal/chunker"
	"github.com/i5heu/ouroboros-db/internal/erasure"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageFlow(t *testing.T) {
	// 1. Setup
	ctx := context.Background()

	// BadgerDB file-based in temp dir
	opts := badger.DefaultOptions(t.TempDir()).
		WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer db.Close()

	bs := blockstore.NewBlockStore(db)

	// 2. Data Chunking
	// Create random data (larger than one chunk size 256KB)
	dataSize := 1024 * 1024 // 1MB
	originalData := make([]byte, dataSize)
	_, err = rand.Read(originalData)
	require.NoError(t, err)

	chnk := chunker.NewChunker(bytes.NewReader(originalData))

	var chunks [][]byte
	for {
		chunkData, err := chnk.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		chunks = append(chunks, chunkData)
	}
	require.NotEmpty(t, chunks)
	t.Logf("Generated %d chunks", len(chunks))

	// 3. Create Block (Simulate DistributedWAL sealing)
	// We'll manually create a Block with "encrypted" chunks (just wrapped)
	var sealedChunks []model.SealedChunk
	var dataSection []byte
	chunkIndex := make(map[hash.Hash]model.ChunkRegion)

	offset := uint32(0)
	for _, chunkBytes := range chunks {
		// "Encrypt"
		sealedHash := hash.HashBytes(chunkBytes) // Fake encrypted hash
		chunkHash := hash.HashBytes(chunkBytes)

		// Create sealed chunk struct
		sc := model.SealedChunk{
			ChunkHash:        chunkHash,
			SealedHash:       sealedHash,
			EncryptedContent: chunkBytes, // Use cleartext as encrypted for test
			Nonce:            make([]byte, 12),
			OriginalSize:     len(chunkBytes),
		}

		serializedSC, err := carrier.Serialize(sc)
		require.NoError(t, err)

		dataSection = append(dataSection, serializedSC...)

		// Update index
		length := uint32(len(serializedSC))
		chunkIndex[chunkHash] = model.ChunkRegion{
			ChunkHash: chunkHash,
			Offset:    offset,
			Length:    length,
		}
		offset += length
		sealedChunks = append(sealedChunks, sc)
	}

	blockHash := hash.HashBytes(dataSection) // Simple hash for block

	block := model.Block{
		Hash: blockHash,
		Header: model.BlockHeader{
			Version:        1,
			Created:        time.Now().UnixMilli(),
			ChunkCount:     uint32(len(sealedChunks)),
			TotalSize:      uint32(len(dataSection)),
			RSDataSlices:   4,
			RSParitySlices: 2,
		},
		DataSection: dataSection,
		ChunkIndex:  chunkIndex,
		VertexIndex: make(map[hash.Hash]model.VertexRegion),
		KeyRegistry: make(map[hash.Hash][]model.KeyEntry),
	}

	// 4. Store Block
	err = bs.StoreBlock(ctx, block)
	require.NoError(t, err)

	// 5. Retrieve Block
	retrievedBlock, err := bs.GetBlock(ctx, blockHash)
	require.NoError(t, err)
	assert.Equal(t, block.Hash, retrievedBlock.Hash)
	assert.Equal(t, block.Header, retrievedBlock.Header)
	assert.Equal(t, len(block.DataSection), len(retrievedBlock.DataSection))

	// 6. Test Region Retrieval
	// Pick a random chunk
	targetChunk := sealedChunks[1]
	region := retrievedBlock.ChunkIndex[targetChunk.ChunkHash]

	retrievedSC, err := bs.GetSealedChunkByRegion(ctx, blockHash, region)
	require.NoError(t, err)
	assert.Equal(t, targetChunk.ChunkHash, retrievedSC.ChunkHash)
	assert.Equal(t, targetChunk.EncryptedContent, retrievedSC.EncryptedContent)

	// 7. Erasure Coding (Slice Block)
	// We serialize the entire block to slice it
	blockBytes, err := carrier.Serialize(block)
	require.NoError(t, err)

	dataShards := 4
	parityShards := 2
	enc, err := erasure.NewErasureEncoder(dataShards, parityShards)
	require.NoError(t, err)

	shards, err := enc.Encode(blockBytes)
	require.NoError(t, err)
	assert.Equal(t, dataShards+parityShards, len(shards))

	// 8. Store Slices
	var slices []model.BlockSlice
	for i, shard := range shards {
		sliceHash := hash.HashBytes(shard)
		slice := model.BlockSlice{
			Hash:           sliceHash,
			BlockHash:      blockHash,
			RSSliceIndex:   uint8(i),
			RSDataSlices:   uint8(dataShards),
			RSParitySlices: uint8(parityShards),
			Payload:        shard,
		}
		err := bs.StoreBlockSlice(ctx, slice)
		require.NoError(t, err)
		slices = append(slices, slice)
	}

	// 9. List Slices
	listedSlices, err := bs.ListBlockSlices(ctx, blockHash)
	require.NoError(t, err)
	assert.Equal(t, len(slices), len(listedSlices))

	// 10. Reconstruct Block from Slices
	// Simulate missing shards (remove 2, since we have 2 parity)
	// Make a copy of shards
	reconstructShards := make([][]byte, dataShards+parityShards)
	for i := 0; i < len(shards); i++ {
		reconstructShards[i] = shards[i]
	}

	// Delete 2 shards (e.g., index 1 and 4)
	reconstructShards[1] = nil
	reconstructShards[4] = nil

	err = enc.Reconstruct(reconstructShards)
	require.NoError(t, err)

	// Verify reconstructed shards match original
	assert.Equal(t, shards[1], reconstructShards[1])
	assert.Equal(t, shards[4], reconstructShards[4])

	// Join back to block bytes
	joinedBytes, err := enc.Join(reconstructShards, len(blockBytes))
	require.NoError(t, err)
	assert.Equal(t, blockBytes, joinedBytes)

	// Deserialize back to Block
	reconstructedBlock, err := carrier.Deserialize[model.Block](joinedBytes)
	require.NoError(t, err)
	assert.Equal(t, block.Hash, reconstructedBlock.Hash)
}
