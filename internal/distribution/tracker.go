// Package distribution implements block distribution tracking for OuroborosDB.
package distribution

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/distribution"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

const (
	// prefixDistribution is the BadgerDB key prefix for distribution records.
	prefixDistribution = "dist:block:"
	// prefixMetadata is the BadgerDB key prefix for block metadata.
	prefixMetadata = "dist:meta:"
)

// DefaultBlockDistributionTracker implements BlockDistributionTracker using
// BadgerDB.
type DefaultBlockDistributionTracker struct {
	db     *badger.DB
	config model.DistributionConfig
	mu     sync.RWMutex

	// active holds in-progress distributions for fast access.
	active map[hash.Hash]*model.BlockDistributionRecord
}

// NewBlockDistributionTracker creates a new tracker instance.
func NewBlockDistributionTracker(
	db *badger.DB,
	config model.DistributionConfig,
) *DefaultBlockDistributionTracker {
	if config.MinNodesRequired == 0 {
		config.MinNodesRequired = 3
	}
	if config.DistributionTimeout == 0 {
		config.DistributionTimeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	tracker := &DefaultBlockDistributionTracker{
		db:     db,
		config: config,
		active: make(map[hash.Hash]*model.BlockDistributionRecord),
	}

	// Load any pending distributions from disk on startup
	tracker.loadPendingDistributions()

	return tracker
}

// StartDistribution begins tracking a new block distribution.
func (t *DefaultBlockDistributionTracker) StartDistribution(
	ctx context.Context,
	block model.Block,
	walKeys [][]byte,
) (*model.BlockDistributionRecord, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	record := &model.BlockDistributionRecord{
		BlockHash:            block.Hash,
		State:                model.BlockStatePending,
		CreatedAt:            now,
		UpdatedAt:            now,
		SliceConfirmations:   make(map[hash.Hash][]string),
		TotalSlices:          block.Header.RSDataSlices + block.Header.RSParitySlices,
		UniqueNodesConfirmed: 0,
		RetryCount:           0,
		WALKeys:              walKeys,
	}

	// Store block metadata for later queries
	metadata := t.extractMetadata(block)
	if err := t.persistMetadata(&metadata); err != nil {
		return nil, fmt.Errorf("persist block metadata: %w", err)
	}

	// Persist distribution record
	if err := t.persistRecord(record); err != nil {
		return nil, fmt.Errorf("persist distribution record: %w", err)
	}

	t.active[block.Hash] = record
	return record, nil
}

// extractMetadata creates BlockMetadata from a Block.
func (t *DefaultBlockDistributionTracker) extractMetadata(
	block model.Block,
) model.BlockMetadata {
	chunkHashes := make([]hash.Hash, 0, len(block.ChunkIndex))
	for h := range block.ChunkIndex {
		chunkHashes = append(chunkHashes, h)
	}

	vertexHashes := make([]hash.Hash, 0, len(block.VertexIndex))
	for h := range block.VertexIndex {
		vertexHashes = append(vertexHashes, h)
	}

	keyEntryKeys := make([]string, 0)
	for chunkHash, keys := range block.KeyRegistry {
		for _, ke := range keys {
			// Match the WAL key format: wal:key:<chunkHash>:<pubKeyHash>
			keyStr := fmt.Sprintf(
				"wal:key:%s:%s",
				chunkHash.String(),
				ke.PubKeyHash.String(),
			)
			keyEntryKeys = append(keyEntryKeys, keyStr)
		}
	}

	return model.BlockMetadata{
		BlockHash:    block.Hash,
		ChunkHashes:  chunkHashes,
		VertexHashes: vertexHashes,
		KeyEntryKeys: keyEntryKeys,
		CreatedAt:    block.Header.Created,
	}
}

// RecordSliceConfirmation records that a node confirmed receipt of a slice.
func (t *DefaultBlockDistributionTracker) RecordSliceConfirmation(
	ctx context.Context,
	blockHash hash.Hash,
	sliceHash hash.Hash,
	nodeID string,
) (bool, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	record, exists := t.active[blockHash]
	if !exists {
		// Try loading from disk
		var err error
		record, err = t.loadRecord(blockHash)
		if err != nil {
			return false, fmt.Errorf(
				"no active distribution for block %s: %w",
				blockHash,
				err,
			)
		}
		t.active[blockHash] = record
	}

	// Add confirmation if not already present
	nodes := record.SliceConfirmations[sliceHash]
	for _, n := range nodes {
		if n == nodeID {
			// Already confirmed by this node
			return record.State == model.BlockStateDistributed, nil
		}
	}
	record.SliceConfirmations[sliceHash] = append(nodes, nodeID)

	// Count unique nodes across all slices
	uniqueNodes := make(map[string]bool)
	for _, confirmations := range record.SliceConfirmations {
		for _, n := range confirmations {
			uniqueNodes[n] = true
		}
	}
	record.UniqueNodesConfirmed = len(uniqueNodes)
	record.UpdatedAt = time.Now()

	// Check if we've reached the threshold
	distributed := record.UniqueNodesConfirmed >= t.config.MinNodesRequired
	if distributed && record.State != model.BlockStateDistributed {
		record.State = model.BlockStateDistributed
	} else if record.State == model.BlockStatePending {
		record.State = model.BlockStateDistributing
	}

	// Persist updated record
	if err := t.persistRecord(record); err != nil {
		return false, fmt.Errorf("persist updated record: %w", err)
	}

	return distributed, nil
}

// GetDistributionState returns the current state of a block's distribution.
func (t *DefaultBlockDistributionTracker) GetDistributionState(
	ctx context.Context,
	blockHash hash.Hash,
) (*model.BlockDistributionRecord, error) {
	t.mu.RLock()
	if record, exists := t.active[blockHash]; exists {
		t.mu.RUnlock()
		return record, nil
	}
	t.mu.RUnlock()

	// Try loading from disk
	return t.loadRecord(blockHash)
}

// GetPendingDistributions returns all blocks in Pending or Distributing state.
func (t *DefaultBlockDistributionTracker) GetPendingDistributions(
	ctx context.Context,
) ([]*model.BlockDistributionRecord, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]*model.BlockDistributionRecord, 0, len(t.active))
	for _, record := range t.active {
		if record.State == model.BlockStatePending ||
			record.State == model.BlockStateDistributing {
			result = append(result, record)
		}
	}
	return result, nil
}

// MarkDistributed explicitly marks a block as successfully distributed.
func (t *DefaultBlockDistributionTracker) MarkDistributed(
	ctx context.Context,
	blockHash hash.Hash,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	record, exists := t.active[blockHash]
	if !exists {
		return fmt.Errorf("no active distribution for block %s", blockHash)
	}

	record.State = model.BlockStateDistributed
	record.UpdatedAt = time.Now()

	if err := t.persistRecord(record); err != nil {
		return err
	}

	// Remove from active cache since it's complete
	delete(t.active, blockHash)

	return nil
}

// MarkFailed marks a block distribution as failed.
func (t *DefaultBlockDistributionTracker) MarkFailed(
	ctx context.Context,
	blockHash hash.Hash,
	reason string,
) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	record, exists := t.active[blockHash]
	if !exists {
		return fmt.Errorf("no active distribution for block %s", blockHash)
	}

	record.State = model.BlockStateFailed
	record.UpdatedAt = time.Now()

	if err := t.persistRecord(record); err != nil {
		return err
	}

	// Keep in active for manual retry/investigation
	return nil
}

// GetDistributedBlocksSince returns block hashes distributed after a timestamp.
func (t *DefaultBlockDistributionTracker) GetDistributedBlocksSince(
	ctx context.Context,
	since int64,
) ([]hash.Hash, error) {
	var hashes []hash.Hash
	sinceTime := time.UnixMilli(since)

	err := t.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(prefixDistribution)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(v []byte) error {
				var record model.BlockDistributionRecord
				if err := gob.NewDecoder(
					bytes.NewReader(v),
				).Decode(
					&record,
				); err != nil {
					return nil // Skip corrupted records
				}

				if record.State == model.BlockStateDistributed &&
					record.UpdatedAt.After(sinceTime) {
					hashes = append(hashes, record.BlockHash)
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return hashes, err
}

// GetBlockMetadata returns metadata for a distributed block.
func (t *DefaultBlockDistributionTracker) GetBlockMetadata(
	ctx context.Context,
	blockHash hash.Hash,
) (*model.BlockMetadata, error) {
	key := []byte(prefixMetadata + blockHash.String())
	var metadata model.BlockMetadata

	err := t.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			return gob.NewDecoder(bytes.NewReader(v)).Decode(&metadata)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("get block metadata: %w", err)
	}
	return &metadata, nil
}

// Persistence helpers

func (t *DefaultBlockDistributionTracker) persistRecord(
	record *model.BlockDistributionRecord,
) error {
	key := []byte(prefixDistribution + record.BlockHash.String())

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(record); err != nil {
		return fmt.Errorf("encode distribution record: %w", err)
	}

	return t.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, buf.Bytes())
	})
}

func (t *DefaultBlockDistributionTracker) persistMetadata(
	metadata *model.BlockMetadata,
) error {
	key := []byte(prefixMetadata + metadata.BlockHash.String())

	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(metadata); err != nil {
		return fmt.Errorf("encode block metadata: %w", err)
	}

	return t.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, buf.Bytes())
	})
}

func (t *DefaultBlockDistributionTracker) loadRecord(
	blockHash hash.Hash,
) (*model.BlockDistributionRecord, error) {
	key := []byte(prefixDistribution + blockHash.String())
	var record model.BlockDistributionRecord

	err := t.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(v []byte) error {
			return gob.NewDecoder(bytes.NewReader(v)).Decode(&record)
		})
	})
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (t *DefaultBlockDistributionTracker) loadPendingDistributions() {
	_ = t.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		prefix := []byte(prefixDistribution)
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			_ = item.Value(func(v []byte) error {
				var record model.BlockDistributionRecord
				if err := gob.NewDecoder(
					bytes.NewReader(v),
				).Decode(
					&record,
				); err != nil {
					return nil // Skip corrupted records
				}

				if record.State == model.BlockStatePending ||
					record.State == model.BlockStateDistributing {
					t.active[record.BlockHash] = &record
				}
				return nil
			})
		}
		return nil
	})
}

// Ensure DefaultBlockDistributionTracker implements the interface.
var _ distribution.BlockDistributionTracker = (*DefaultBlockDistributionTracker)(
	nil,
)
