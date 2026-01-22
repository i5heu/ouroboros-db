// Package wal provides Write-Ahead Log implementations for OuroborosDB.
package wal

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/i5heu/ouroboros-db/pkg/wal"
)

// DefaultBlockSize is the target size for blocks (16MB).
const DefaultBlockSize = 16 * 1024 * 1024

const (
	prefixChunk  = "wal:chunk:"
	prefixVertex = "wal:vertex:"
	prefixKey    = "wal:key:"
)

// DefaultDistributedWAL implements the DistributedWAL interface.
type DefaultDistributedWAL struct {
	db         *badger.DB
	bufferSize int64
	targetSize int64
}

// NewDistributedWAL creates a new DefaultDistributedWAL instance.
func NewDistributedWAL(db *badger.DB) *DefaultDistributedWAL {
	w := &DefaultDistributedWAL{
		db:         db,
		targetSize: DefaultBlockSize,
	}
	w.recalcBufferSize()
	return w
}

func (w *DefaultDistributedWAL) recalcBufferSize() {
	w.bufferSize = 0
	_ = w.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("wal:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())

			// We only count size for chunks and vertices as per original logic,
			// but we should probably count everything.
			// For now, let's stick to approximate size of payloads.

			if strings.HasPrefix(key, prefixChunk) {
				valSize := item.ValueSize()
				// Approximate, we'd need to read to be exact about "content" size vs
				// "entry" size
				// but BadgerDB ValueSize is close enough for a WAL buffer limit.
				w.bufferSize += int64(valSize)
			} else if strings.HasPrefix(key, prefixVertex) {
				w.bufferSize += int64(item.ValueSize())
			}
			// Keys are small, maybe ignore or add small constant?
		}
		return nil
	})
}

// AppendChunk adds a sealed chunk to the buffer.
func (w *DefaultDistributedWAL) AppendChunk(
	ctx context.Context,
	chunk model.SealedChunk,
) error {
	key := []byte(prefixChunk + chunk.ChunkHash.String())
	data, err := serialize(chunk)
	if err != nil {
		return fmt.Errorf("serialize chunk: %w", err)
	}

	err = w.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("persist chunk: %w", err)
	}

	w.bufferSize += int64(len(data))
	return nil
}

// AppendVertex adds a vertex to the buffer.
func (w *DefaultDistributedWAL) AppendVertex(
	ctx context.Context,
	vertex model.Vertex,
) error {
	key := []byte(prefixVertex + vertex.Hash.String())
	data, err := serialize(vertex)
	if err != nil {
		return fmt.Errorf("serialize vertex: %w", err)
	}

	err = w.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("persist vertex: %w", err)
	}

	w.bufferSize += int64(len(data))
	return nil
}

// AppendKeyEntry adds a key entry to the buffer.
func (w *DefaultDistributedWAL) AppendKeyEntry(
	ctx context.Context,
	keyEntry model.KeyEntry,
) error {
	// composite key: wal:key:<chunkHash>:<pubKeyHash>
	keyStr := fmt.Sprintf(
		"%s%s:%s",
		prefixKey,
		keyEntry.ChunkHash.String(),
		keyEntry.PubKeyHash.String(),
	)
	key := []byte(keyStr)

	data, err := serialize(keyEntry)
	if err != nil {
		return fmt.Errorf("serialize key entry: %w", err)
	}

	err = w.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("persist key entry: %w", err)
	}

	return nil
}

// SealBlock creates a new block from the buffered items.
//
// IMPORTANT: This method does NOT delete WAL entries. The caller must call
// ClearBlock() after confirming the block has been successfully distributed
// to the required number of nodes.
//
// Returns:
//   - The sealed Block containing all buffered items
//   - The WAL keys that should be passed to ClearBlock() after distribution
//   - Error if block creation fails
func (w *DefaultDistributedWAL) SealBlock(
	ctx context.Context,
) (model.Block, [][]byte, error) {
	var chunks []model.SealedChunk
	var vertices []model.Vertex
	keyEntries := make(map[hash.Hash][]model.KeyEntry)
	var walKeys [][]byte

	err := w.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte("wal:")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.Key()
			keyStr := string(k)

			// Make a copy of the key for later deletion via ClearBlock
			keyCopy := make([]byte, len(k))
			copy(keyCopy, k)
			walKeys = append(walKeys, keyCopy)

			err := item.Value(func(v []byte) error {
				if strings.HasPrefix(keyStr, prefixChunk) {
					var c model.SealedChunk
					if err := deserialize(v, &c); err != nil {
						return err
					}
					chunks = append(chunks, c)
				} else if strings.HasPrefix(keyStr, prefixVertex) {
					var vtx model.Vertex
					if err := deserialize(v, &vtx); err != nil {
						return err
					}
					vertices = append(vertices, vtx)
				} else if strings.HasPrefix(keyStr, prefixKey) {
					var ke model.KeyEntry
					if err := deserialize(v, &ke); err != nil {
						return err
					}
					keyEntries[ke.ChunkHash] = append(keyEntries[ke.ChunkHash], ke)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return model.Block{}, nil, fmt.Errorf("iterate wal: %w", err)
	}

	if len(chunks) == 0 && len(vertices) == 0 {
		return model.Block{}, nil, fmt.Errorf("wal: no data to seal")
	}

	block := w.createBlock(chunks, vertices, keyEntries)

	// DO NOT delete WAL entries here. The caller must call ClearBlock()
	// after confirming the block has been distributed to sufficient nodes.
	// This ensures crash safety - if we crash before distribution, the
	// WAL entries are preserved and can be re-sealed on restart.

	return block, walKeys, nil
}

// ClearBlock removes WAL entries for a successfully distributed block.
//
// This should only be called after confirming the block has been distributed
// to the required number of nodes (typically 3+). The walKeys parameter
// should come from the SealBlock() return value.
//
// Parameters:
//   - walKeys: The keys returned by SealBlock() for this block
//
// Returns:
//   - Error if the WAL entries cannot be deleted
func (w *DefaultDistributedWAL) ClearBlock(
	ctx context.Context,
	walKeys [][]byte,
) error {
	err := w.db.Update(func(txn *badger.Txn) error {
		for _, k := range walKeys {
			if err := txn.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("clear wal entries: %w", err)
	}

	// Recalculate buffer size after clearing entries
	w.recalcBufferSize()
	return nil
}

// GetBufferSize returns the current size of buffered data.
func (w *DefaultDistributedWAL) GetBufferSize() int64 {
	return w.bufferSize
}

// Flush forces the creation of a block even if the buffer isn't full.
//
// Like SealBlock(), this does NOT delete WAL entries. The caller must call
// ClearBlock() after confirming the block has been successfully distributed.
func (w *DefaultDistributedWAL) Flush(
	ctx context.Context,
) (model.Block, [][]byte, error) {
	return w.SealBlock(ctx)
}

func (w *DefaultDistributedWAL) createBlock(
	chunks []model.SealedChunk,
	vertices []model.Vertex,
	keyEntries map[hash.Hash][]model.KeyEntry,
) model.Block {
	// Serialize chunks into DataSection
	var dataSection []byte
	chunkIndex := make(map[hash.Hash]model.ChunkRegion)
	offset := uint32(0)
	for _, chunk := range chunks {
		if len(chunk.EncryptedContent) > math.MaxUint32 {
			// This should practically definitely not happen given chunk sizes,
			// but we handle it just in case
			continue
		}
		//nolint:gosec // range check above ensures safety
		length := uint32(len(chunk.EncryptedContent))
		chunkIndex[chunk.ChunkHash] = model.ChunkRegion{
			ChunkHash: chunk.ChunkHash,
			Offset:    offset,
			Length:    length,
		}
		dataSection = append(dataSection, chunk.EncryptedContent...)
		offset += length
	}

	// Serialize vertices into VertexSection (simplified)
	var vertexSection []byte
	vertexIndex := make(map[hash.Hash]model.VertexRegion)
	offset = 0 // Offset within VertexSection
	for _, vertex := range vertices {
		// Use gob for vertex serialization in the block for now, or just hash as
		// per original The original code used `vertex.Hash[:]` which is just the
		// hash, not the vertex data.
		// That seems wrong if we want to store the vertex itself.
		// The `model.VertexRegion` implies we store the vertex data.
		// Let's serialize the whole vertex using gob.

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		_ = enc.Encode(vertex) // Ignoring error for simplicity in this helper
		vertexBytes := buf.Bytes()

		if len(vertexBytes) > math.MaxUint32 {
			continue
		}
		//nolint:gosec // strict check above ensures safety
		length := uint32(len(vertexBytes))
		vertexIndex[vertex.Hash] = model.VertexRegion{
			VertexHash: vertex.Hash,
			Offset:     offset,
			Length:     length,
		}
		vertexSection = append(vertexSection, vertexBytes...)
		offset += length
	}

	// Calculate block hash
	hashInput := append(dataSection, vertexSection...)
	blockHash := hash.HashBytes(hashInput)

	chunkCount := uint32(len(chunks))                          //nolint:gosec
	vertexCount := uint32(len(vertices))                       //nolint:gosec
	totalSize := uint32(len(dataSection) + len(vertexSection)) //nolint:gosec

	return model.Block{
		Hash: blockHash,
		Header: model.BlockHeader{
			Version:     1,
			Created:     time.Now().UnixMilli(),
			ChunkCount:  chunkCount,
			VertexCount: vertexCount,
			TotalSize:   totalSize,
		},
		DataSection:   dataSection,
		VertexSection: vertexSection,
		ChunkIndex:    chunkIndex,
		VertexIndex:   vertexIndex,
		KeyRegistry:   keyEntries,
	}
}

func serialize(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(v)
	return buf.Bytes(), err
}

func deserialize(data []byte, v interface{}) error {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)
	return dec.Decode(v)
}

// Ensure DefaultDistributedWAL implements the DistributedWAL interface.
var _ wal.DistributedWAL = (*DefaultDistributedWAL)(nil)

// DefaultDeletionWAL implements the DeletionWAL interface.
type DefaultDeletionWAL struct {
	pendingDeletions []hash.Hash
}

// NewDeletionWAL creates a new DefaultDeletionWAL instance.
func NewDeletionWAL() *DefaultDeletionWAL {
	return &DefaultDeletionWAL{
		pendingDeletions: make([]hash.Hash, 0),
	}
}

// LogDeletion records a hash for deletion.
func (w *DefaultDeletionWAL) LogDeletion(
	ctx context.Context,
	h hash.Hash,
) error {
	w.pendingDeletions = append(w.pendingDeletions, h)
	return nil
}

// ProcessDeletions processes pending deletions.
func (w *DefaultDeletionWAL) ProcessDeletions(ctx context.Context) error {
	// Implementation will actually delete the data
	// This is a placeholder for the actual implementation
	w.pendingDeletions = make([]hash.Hash, 0)
	return nil
}

// GetPendingDeletions returns the list of hashes pending deletion.
func (w *DefaultDeletionWAL) GetPendingDeletions(
	ctx context.Context,
) ([]hash.Hash, error) {
	result := make([]hash.Hash, len(w.pendingDeletions))
	copy(result, w.pendingDeletions)
	return result, nil
}

// Ensure DefaultDeletionWAL implements the DeletionWAL interface.
var _ wal.DeletionWAL = (*DefaultDeletionWAL)(nil)
