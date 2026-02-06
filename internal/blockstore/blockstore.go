// Package blockstore provides block storage implementations for OuroborosDB.
package blockstore

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/i5heu/ouroboros-db/pkg/storage"
)

const (
	// Prefix definitions for BadgerDB keys
	prefixBlock      = "blk:b:"
	prefixSlice      = "blk:s:"
	prefixBlockSlice = "blk:bs:" // Index: blockHash -> sliceHash
)

// BadgerBlockStore implements the BlockStore interface using BadgerDB.
type BadgerBlockStore struct {
	db *badger.DB
}

// NewBlockStore creates a new BadgerBlockStore instance.
func NewBlockStore(db *badger.DB) *BadgerBlockStore {
	return &BadgerBlockStore{
		db: db,
	}
}

// StoreBlock persists a block to storage.
func (s *BadgerBlockStore) StoreBlock(
	ctx context.Context,
	block model.Block,
) error {
	if block.Hash == (hash.Hash{}) {
		return fmt.Errorf("blockstore: block hash is required")
	}

	data, err := carrier.Serialize(block)
	if err != nil {
		return fmt.Errorf("serialize block: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		key := []byte(prefixBlock + block.Hash.String())
		// Set TTL or other options if needed, but blocks are permanent for now
		return txn.Set(key, data)
	})
	if err != nil {
		return err
	}

	// Debug log that we've stored a block
	slog.DebugContext(ctx, "blockstore: stored block", "blockHash", block.Hash.String(), "vertexCount", block.Header.VertexCount, "chunkCount", block.Header.ChunkCount)
	return nil
}

// GetBlock retrieves a block by its hash.
func (s *BadgerBlockStore) GetBlock(
	ctx context.Context,
	blockHash hash.Hash,
) (model.Block, error) {
	var block model.Block

	err := s.db.View(func(txn *badger.Txn) error {
		key := []byte(prefixBlock + blockHash.String())
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return fmt.Errorf("blockstore: block %s not found", blockHash)
			}
			return err
		}

		return item.Value(func(val []byte) error {
			var err error
			block, err = carrier.Deserialize[model.Block](val)
			return err
		})
	})

	return block, err
}

// DeleteBlock removes a block from storage.
func (s *BadgerBlockStore) DeleteBlock(
	ctx context.Context,
	blockHash hash.Hash,
) error {
	return s.db.Update(func(txn *badger.Txn) error {
		key := []byte(prefixBlock + blockHash.String())
		return txn.Delete(key)
	})
}

// StoreBlockSlice persists a block slice to storage.
func (s *BadgerBlockStore) StoreBlockSlice(
	ctx context.Context,
	slice model.BlockSlice,
) error {
	if slice.Hash == (hash.Hash{}) {
		return fmt.Errorf("blockstore: slice hash is required")
	}

	data, err := carrier.Serialize(slice)
	if err != nil {
		return fmt.Errorf("serialize slice: %w", err)
	}

	// Serialize hash for index
	hashData, err := carrier.Serialize(slice.Hash)
	if err != nil {
		return fmt.Errorf("serialize slice hash: %w", err)
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		// Store the slice
		sliceKey := []byte(prefixSlice + slice.Hash.String())
		if err := txn.Set(sliceKey, data); err != nil {
			return err
		}

		// Update index: blockHash -> sliceHash
		indexKey := []byte(prefixBlockSlice + slice.BlockHash.String() + ":" + slice.Hash.String())
		// We store the serialized hash as value
		return txn.Set(indexKey, hashData)
	})
	if err != nil {
		return err
	}

	// Debug log that we've stored a block slice
	slog.DebugContext(ctx, "blockstore: stored block slice", "sliceHash", slice.Hash.String(), "blockHash", slice.BlockHash.String())
	return nil
}

// GetBlockSlice retrieves a block slice by its hash.
func (s *BadgerBlockStore) GetBlockSlice(
	ctx context.Context,
	sliceHash hash.Hash,
) (model.BlockSlice, error) {
	var slice model.BlockSlice

	err := s.db.View(func(txn *badger.Txn) error {
		key := []byte(prefixSlice + sliceHash.String())
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return fmt.Errorf("blockstore: slice %s not found", sliceHash)
			}
			return err
		}

		return item.Value(func(val []byte) error {
			var err error
			slice, err = carrier.Deserialize[model.BlockSlice](val)
			return err
		})
	})

	return slice, err
}

// ListBlockSlices returns all slices for a given block.
func (s *BadgerBlockStore) ListBlockSlices(
	ctx context.Context,
	blockHash hash.Hash,
) ([]model.BlockSlice, error) {
	var slices []model.BlockSlice

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = true // We need values (serialized hash)
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte(prefixBlockSlice + blockHash.String() + ":")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()

			// Get slice hash from value
			err := item.Value(func(val []byte) error {
				sliceHash, err := carrier.Deserialize[hash.Hash](val)
				if err != nil {
					return err
				}

				// Now fetch the actual slice
				// Note: Nested transaction usage or separate retrieval?
				// We are in a View, so we can access other keys using same txn?
				// Yes, txn is valid.

				sliceKey := []byte(prefixSlice + sliceHash.String())
				sliceItem, err := txn.Get(sliceKey)
				if err != nil {
					// Slice missing but in index?
					return err
				}

				return sliceItem.Value(func(sliceVal []byte) error {
					slice, err := carrier.Deserialize[model.BlockSlice](sliceVal)
					if err != nil {
						return err
					}
					slices = append(slices, slice)
					return nil
				})
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	return slices, err
}

// GetSealedChunkByRegion retrieves a sealed chunk using region-based lookup.
func (s *BadgerBlockStore) GetSealedChunkByRegion(
	ctx context.Context,
	blockHash hash.Hash,
	region model.ChunkRegion,
) (model.SealedChunk, error) {
	// Optimization: This loads the entire block.
	// Future optimization: Store DataSection separately or use efficient value access
	// if Badger supports partial value reads (not natively supported in simple API).
	block, err := s.GetBlock(ctx, blockHash)
	if err != nil {
		return model.SealedChunk{}, err
	}

	if int(region.Offset+region.Length) > len(block.DataSection) {
		return model.SealedChunk{}, fmt.Errorf(
			"blockstore: region exceeds block data section",
		)
	}

	// We have the block, we can construct the SealedChunk.
	// Note: We only return the encrypted content and hash.
	// The SealedChunk struct has more fields (Nonce, SealedHash, OriginalSize)
	// which are serialized inside the DataSection bytes.
	//
	// Wait! The DataSection contains "Serialized SealedChunks".
	// If the DataSection is just a concatenation of bytes, we need to know format.
	//
	// Looking at pkg/model/block.go:
	// "Serializes buffered SealedChunks into DataSection"
	//
	// If it's just raw bytes of the *content*, then we can't reconstruct SealedChunk easily
	// without the metadata (Nonce, etc.).
	//
	// However, usually "Serialized SealedChunk" means the SealedChunk struct serialized.
	// If so, region points to the serialized bytes of a SealedChunk.
	// So we should deserialize it.

	chunkBytes := block.DataSection[region.Offset : region.Offset+region.Length]
	sealedChunk, err := carrier.Deserialize[model.SealedChunk](chunkBytes)
	if err != nil {
		return model.SealedChunk{}, fmt.Errorf("deserialize sealed chunk: %w", err)
	}

	// Verify hash matches?
	if sealedChunk.ChunkHash != region.ChunkHash {
		// This might happen if region index is wrong
		return model.SealedChunk{}, fmt.Errorf("chunk hash mismatch in storage")
	}

	return sealedChunk, nil
}

// GetVertexByRegion retrieves a vertex using region-based lookup.
func (s *BadgerBlockStore) GetVertexByRegion(
	ctx context.Context,
	blockHash hash.Hash,
	region model.VertexRegion,
) (model.Vertex, error) {
	block, err := s.GetBlock(ctx, blockHash)
	if err != nil {
		return model.Vertex{}, err
	}

	if int(region.Offset+region.Length) > len(block.VertexSection) {
		return model.Vertex{}, fmt.Errorf(
			"blockstore: region exceeds block vertex section",
		)
	}

	vertexBytes := block.VertexSection[region.Offset : region.Offset+region.Length]
	vertex, err := carrier.Deserialize[model.Vertex](vertexBytes)
	if err != nil {
		return model.Vertex{}, fmt.Errorf("deserialize vertex: %w", err)
	}

	return vertex, nil
}

// Ensure BadgerBlockStore implements the BlockStore interface.
var _ storage.BlockStore = (*BadgerBlockStore)(nil)
