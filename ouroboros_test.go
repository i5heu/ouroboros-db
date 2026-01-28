package ouroboros

import (
	"context"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/internal/blockstore"
	"github.com/i5heu/ouroboros-db/internal/wal"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetStats_CountsBlockContentsAfterFlush verifies that GetStats correctly
// counts vertices, chunks, and keys from both WAL entries and sealed blocks.
func TestGetStats_CountsBlockContentsAfterFlush(t *testing.T) {
	// Setup: in-memory Badger
	opts := badger.DefaultOptions("")
	opts = opts.WithInMemory(true)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer db.Close()

	bs := blockstore.NewBlockStore(db)
	w := wal.NewDistributedWAL(db, bs, nil)

	// Create test data: 3 chunks, 2 vertices, 2 key entries
	chunks := []model.SealedChunk{
		{ChunkHash: hash.HashBytes([]byte("chunk1")), EncryptedContent: []byte("data1")},
		{ChunkHash: hash.HashBytes([]byte("chunk2")), EncryptedContent: []byte("data2")},
		{ChunkHash: hash.HashBytes([]byte("chunk3")), EncryptedContent: []byte("data3")},
	}
	vertices := []model.Vertex{
		{Hash: hash.HashBytes([]byte("vertex1"))},
		{Hash: hash.HashBytes([]byte("vertex2"))},
	}
	keyEntries := []model.KeyEntry{
		{ChunkHash: chunks[0].ChunkHash, PubKeyHash: hash.HashBytes([]byte("pubkey1")), EncapsulatedAESKey: []byte("key1")},
		{ChunkHash: chunks[1].ChunkHash, PubKeyHash: hash.HashBytes([]byte("pubkey2")), EncapsulatedAESKey: []byte("key2")},
	}

	ctx := context.Background()

	// Append items to WAL
	for _, c := range chunks {
		require.NoError(t, w.AppendChunk(ctx, c))
	}
	for _, v := range vertices {
		require.NoError(t, w.AppendVertex(ctx, v))
	}
	for _, ke := range keyEntries {
		require.NoError(t, w.AppendKeyEntry(ctx, ke))
	}

	// Create OuroborosDB wrapper for testing GetStats
	odb := &OuroborosDB{db: db}

	// Before flush: stats should count WAL entries
	statsBefore, err := odb.GetStats()
	require.NoError(t, err)
	assert.Equal(t, int64(3), statsBefore.Chunks, "should count 3 chunks from WAL")
	assert.Equal(t, int64(2), statsBefore.Vertices, "should count 2 vertices from WAL")
	assert.Equal(t, int64(2), statsBefore.Keys, "should count 2 keys from WAL")
	assert.Equal(t, int64(0), statsBefore.Blocks, "should have no blocks before seal")

	// Seal the block (flush WAL to block)
	block, walKeys, err := w.SealBlock(ctx)
	require.NoError(t, err)
	require.NotNil(t, block)

	// Store the block
	require.NoError(t, bs.StoreBlock(ctx, block))

	// Clear WAL entries
	require.NoError(t, w.ClearBlock(ctx, walKeys))

	// After flush+clear: stats should count from block contents
	statsAfter, err := odb.GetStats()
	require.NoError(t, err)

	// Block should exist
	assert.Equal(t, int64(1), statsAfter.Blocks, "should have 1 block after seal")

	// Counts should match original data (from block contents now, not WAL)
	assert.Equal(t, int64(3), statsAfter.Chunks, "should count 3 chunks from block contents")
	assert.Equal(t, int64(2), statsAfter.Vertices, "should count 2 vertices from block contents")
	assert.Equal(t, int64(2), statsAfter.Keys, "should count 2 keys from block KeyRegistry")
}

// TestGetStats_CountsMultipleBlocks verifies that GetStats correctly sums
// counts across multiple blocks.
func TestGetStats_CountsMultipleBlocks(t *testing.T) {
	opts := badger.DefaultOptions("")
	opts = opts.WithInMemory(true)
	db, err := badger.Open(opts)
	require.NoError(t, err)
	defer db.Close()

	// Create two blocks directly in Badger
	block1 := model.Block{
		Hash: hash.HashBytes([]byte("block1")),
		Header: model.BlockHeader{
			Version:     1,
			Created:     time.Now().UnixMilli(),
			ChunkCount:  5,
			VertexCount: 3,
		},
		KeyRegistry: map[hash.Hash][]model.KeyEntry{
			hash.HashBytes([]byte("c1")): {{}, {}},    // 2 entries
			hash.HashBytes([]byte("c2")): {{}},        // 1 entry
		},
	}
	block2 := model.Block{
		Hash: hash.HashBytes([]byte("block2")),
		Header: model.BlockHeader{
			Version:     1,
			Created:     time.Now().UnixMilli(),
			ChunkCount:  7,
			VertexCount: 4,
		},
		KeyRegistry: map[hash.Hash][]model.KeyEntry{
			hash.HashBytes([]byte("c3")): {{}, {}, {}}, // 3 entries
		},
	}

	// Serialize and store blocks
	block1Data, err := carrier.Serialize(block1)
	require.NoError(t, err)
	block2Data, err := carrier.Serialize(block2)
	require.NoError(t, err)

	err = db.Update(func(txn *badger.Txn) error {
		if err := txn.Set([]byte("blk:b:"+block1.Hash.String()), block1Data); err != nil {
			return err
		}
		return txn.Set([]byte("blk:b:"+block2.Hash.String()), block2Data)
	})
	require.NoError(t, err)

	// Test GetStats
	odb := &OuroborosDB{db: db}
	stats, err := odb.GetStats()
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.Blocks, "should count 2 blocks")
	assert.Equal(t, int64(12), stats.Chunks, "should count 5+7=12 chunks")
	assert.Equal(t, int64(7), stats.Vertices, "should count 3+4=7 vertices")
	assert.Equal(t, int64(6), stats.Keys, "should count 2+1+3=6 keys")
}
