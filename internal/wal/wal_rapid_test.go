package wal

import (
	"context"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"pgregory.net/rapid"
)

// Generators

func genHash(t *rapid.T) hash.Hash {
	var h hash.Hash
	// Generate 32 random bytes
	bytes := rapid.SliceOfN(rapid.Byte(), 32, 32).Draw(t, "hashBytes")
	copy(h[:], bytes)
	return h
}

func genSealedChunk(t *rapid.T) model.SealedChunk {
	return model.SealedChunk{
		ChunkHash:        rapid.Custom(genHash).Draw(t, "chunkHash"),
		SealedHash:       rapid.Custom(genHash).Draw(t, "sealedHash"),
		EncryptedContent: rapid.SliceOf(rapid.Byte()).Draw(t, "encryptedContent"),
		Nonce:            rapid.SliceOfN(rapid.Byte(), 12, 12).Draw(t, "nonce"),
		OriginalSize:     rapid.Int().Draw(t, "originalSize"),
	}
}

func genVertex(t *rapid.T) model.Vertex {
	return model.Vertex{
		Hash:        rapid.Custom(genHash).Draw(t, "vertexHash"),
		Parent:      rapid.Custom(genHash).Draw(t, "parentHash"),
		Created:     rapid.Int64().Draw(t, "created"),
		ChunkHashes: rapid.SliceOf(rapid.Custom(genHash)).Draw(t, "chunkHashes"),
	}
}

func genKeyEntry(t *rapid.T) model.KeyEntry {
	return model.KeyEntry{
		ChunkHash:          rapid.Custom(genHash).Draw(t, "keyChunkHash"),
		PubKeyHash:         rapid.Custom(genHash).Draw(t, "pubKeyHash"),
		EncapsulatedAESKey: rapid.SliceOf(rapid.Byte()).Draw(t, "encapsulatedKey"),
	}
}

// WALStateMachine defines the state machine for testing the WAL
type WALStateMachine struct {
	// Model state
	expectedChunks   []model.SealedChunk
	expectedVertices []model.Vertex
	expectedKeys     []model.KeyEntry

	// SUT state
	dbPath string
	db     *badger.DB
	wal    *DefaultDistributedWAL
}

// Init initializes the state machine
func (m *WALStateMachine) Init(t *rapid.T) {
	// Create a temp directory for this run
	dir, err := os.MkdirTemp("", "wal-rapid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	m.dbPath = dir

	m.openDB(t)

	m.expectedChunks = make([]model.SealedChunk, 0)
	m.expectedVertices = make([]model.Vertex, 0)
	m.expectedKeys = make([]model.KeyEntry, 0)
}

func (m *WALStateMachine) openDB(t *rapid.T) {
	// Reduce noise by setting logger to nil or error only
	opts := badger.DefaultOptions(m.dbPath).
		WithLoggingLevel(badger.ERROR)

	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to open badger: %v", err)
	}
	m.db = db
	m.wal = NewDistributedWAL(db)
}

func (m *WALStateMachine) Cleanup() {
	if m.db != nil {
		_ = m.db.Close()
	}
	if m.dbPath != "" {
		_ = os.RemoveAll(m.dbPath)
	}
}

// Check consistency
func (m *WALStateMachine) Check(t *rapid.T) {
	size := m.wal.GetBufferSize()
	hasItems := len(m.expectedChunks) > 0 || len(m.expectedVertices) > 0

	if hasItems && size == 0 {
		t.Errorf("Buffer size is 0 but have items (chunks/vertices)")
	}
	if !hasItems && size != 0 {
		t.Errorf("Buffer size is %d but have no items (chunks/vertices)", size)
	}
}

// Action: AppendChunk
func (m *WALStateMachine) AppendChunk(t *rapid.T) {
	chunk := genSealedChunk(t)

	err := m.wal.AppendChunk(context.Background(), chunk)
	if err != nil {
		t.Fatalf("AppendChunk failed: %v", err)
	}

	m.expectedChunks = append(m.expectedChunks, chunk)
}

// Action: AppendVertex
func (m *WALStateMachine) AppendVertex(t *rapid.T) {
	vertex := genVertex(t)

	err := m.wal.AppendVertex(context.Background(), vertex)
	if err != nil {
		t.Fatalf("AppendVertex failed: %v", err)
	}

	m.expectedVertices = append(m.expectedVertices, vertex)
}

// Action: AppendKeyEntry
func (m *WALStateMachine) AppendKeyEntry(t *rapid.T) {
	key := genKeyEntry(t)

	err := m.wal.AppendKeyEntry(context.Background(), key)
	if err != nil {
		t.Fatalf("AppendKeyEntry failed: %v", err)
	}

	m.expectedKeys = append(m.expectedKeys, key)
}

// Action: SealBlock
func (m *WALStateMachine) SealBlock(t *rapid.T) {
	// If empty, SealBlock might fail or return empty. Implementation returns
	// error if empty.
	shouldFail := len(m.expectedChunks) == 0 && len(m.expectedVertices) == 0

	block, walKeys, err := m.wal.SealBlock(context.Background())

	if shouldFail {
		if err == nil {
			t.Fatal("Expected SealBlock to fail on empty buffer")
		}
		return
	}

	if err != nil {
		t.Fatalf("SealBlock failed: %v", err)
	}

	if int(block.Header.ChunkCount) != len(m.expectedChunks) {
		t.Errorf(
			"Block ChunkCount %d != expected %d",
			block.Header.ChunkCount,
			len(m.expectedChunks),
		)
	}
	if int(block.Header.VertexCount) != len(m.expectedVertices) {
		t.Errorf(
			"Block VertexCount %d != expected %d",
			block.Header.VertexCount,
			len(m.expectedVertices),
		)
	}

	// Verify KeyRegistry
	expectedKeyMap := make(map[hash.Hash][]model.KeyEntry)
	countKeys := 0
	for _, k := range m.expectedKeys {
		expectedKeyMap[k.ChunkHash] = append(expectedKeyMap[k.ChunkHash], k)
		countKeys++
	}

	blockKeyCount := 0
	for _, keys := range block.KeyRegistry {
		blockKeyCount += len(keys)
	}
	if blockKeyCount != countKeys {
		t.Errorf("Block KeyCount %d != expected %d", blockKeyCount, countKeys)
	}

	// Verify WAL keys were returned for later clearing
	// Verify WAL keys were returned for later clearing
	hasData := len(m.expectedChunks) > 0 || len(m.expectedVertices) > 0 ||
		len(m.expectedKeys) > 0
	if len(walKeys) == 0 && hasData {
		t.Errorf("SealBlock should return WAL keys when there is data")
	}

	// Simulate distribution confirmation by calling ClearBlock
	if err := m.wal.ClearBlock(context.Background(), walKeys); err != nil {
		t.Fatalf("ClearBlock failed: %v", err)
	}

	// Verify persistence cleared after ClearBlock
	if m.wal.GetBufferSize() != 0 {
		t.Errorf(
			"WAL buffer size not reset after ClearBlock: %d",
			m.wal.GetBufferSize(),
		)
	}

	// Reset expectations
	m.expectedChunks = make([]model.SealedChunk, 0)
	m.expectedVertices = make([]model.Vertex, 0)
	m.expectedKeys = make([]model.KeyEntry, 0)
}

// Action: Restart (Simulate Crash/Restart)
func (m *WALStateMachine) Restart(t *rapid.T) {
	// Close existing
	_ = m.db.Close()
	m.wal = nil

	// Re-open
	opts := badger.DefaultOptions(m.dbPath).WithLoggingLevel(badger.ERROR)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("Failed to re-open badger: %v", err)
	}
	m.db = db
	m.wal = NewDistributedWAL(db)

	hasItems := len(m.expectedChunks) > 0 || len(m.expectedVertices) > 0
	if hasItems && m.wal.GetBufferSize() == 0 {
		t.Errorf("WAL failed to recover buffer size on restart")
	}
}

func TestWALProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		m := &WALStateMachine{}
		m.Init(t)
		defer m.Cleanup()

		t.Repeat(map[string]func(*rapid.T){
			"AppendChunk": func(t *rapid.T) {
				m.AppendChunk(t)
				m.Check(t)
			},
			"AppendVertex": func(t *rapid.T) {
				m.AppendVertex(t)
				m.Check(t)
			},
			"AppendKeyEntry": func(t *rapid.T) {
				m.AppendKeyEntry(t)
				m.Check(t)
			},
			"SealBlock": func(t *rapid.T) {
				m.SealBlock(t)
				m.Check(t)
			},
			"Restart": func(t *rapid.T) {
				m.Restart(t)
				m.Check(t)
			},
		})
	})
}
