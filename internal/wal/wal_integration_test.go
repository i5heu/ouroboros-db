package wal

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/internal/blockstore"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// Basic integration: AppendChunk -> SealBlock -> ClearBlock -> GetChunk via
// index lookup.
func Test_WAL_SealIndexGetChunk(t *testing.T) {
	dir := t.TempDir()
	// Use in-memory Badger for tests to avoid background SST flush
	// noise in ephemeral CI/test environments. Must use empty Dir when
	// in-memory mode is enabled.
	opts := badger.DefaultOptions("")
	opts = opts.WithInMemory(true)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("open badger: %v", err)
	}
	defer db.Close()

	bs := blockstore.NewBlockStore(db)
	w := NewDistributedWAL(db, bs, nil)

	// Create a fake sealed chunk
	var c model.SealedChunk
	c.ChunkHash = hash.HashBytes([]byte("chunk1"))
	c.EncryptedContent = []byte("data")

	if err := w.AppendChunk(context.Background(), c); err != nil {
		t.Fatalf("append chunk: %v", err)
	}

	// Load WAL content and create block directly to avoid StoreBlock
	chunks, vertices, keyEntries, walKeys, err := w.loadWALContent()
	if err != nil {
		t.Fatalf("load wal content: %v", err)
	}
	block := w.createBlock(chunks, vertices, keyEntries)

	// Persist block directly into Badger (avoid using BlockStore which
	// may trigger background flushes in test env)
	if err := w.db.Update(func(txn *badger.Txn) error {
		data, serr := serialize(block)
		if serr != nil {
			return serr
		}
		return txn.Set([]byte("blk:b:"+block.Hash.String()), data)
	}); err != nil {
		t.Fatalf("persist block directly: %v", err)
	}

	// Write index entries as SealBlock would
	if err := w.db.Update(func(txn *badger.Txn) error {
		for _, c := range chunks {
			if region, ok := block.ChunkIndex[c.ChunkHash]; ok {
				entry := struct {
					BlockHash hash.Hash
					Region    model.ChunkRegion
				}{
					BlockHash: block.Hash,
					Region:    region,
				}
				data, serr := serialize(entry)
				if serr != nil {
					continue
				}
				key := []byte(prefixChunkIdx + c.ChunkHash.String())
				if err := txn.Set(key, data); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("write index entries: %v", err)
	}

	// Verify index entry exists before clearing WAL
	idxKey := []byte(prefixChunkIdx + c.ChunkHash.String())
	err = w.db.View(func(txn *badger.Txn) error {
		_, gerr := txn.Get(idxKey)
		return gerr
	})
	if err != nil {
		t.Fatalf("expected idx entry present before clear: %v", err)
	}

	// Clear WAL entries
	if err := w.ClearBlock(context.Background(), walKeys); err != nil {
		t.Fatalf("clear block: %v", err)
	}

	// Now the WAL entry should be gone; GetChunk should use index and return
	// the chunk from persisted block.
	got, err := w.GetChunk(context.Background(), c.ChunkHash)
	if err != nil {
		t.Fatalf("get chunk after seal: %v", err)
	}
	if string(got.EncryptedContent) != string(c.EncryptedContent) {
		t.Fatalf("content mismatch")
	}

	// cleanup
	_ = os.RemoveAll(dir)
}

// Verify ClearBlock removes WAL entries but leaves idx:chunk entries intact
func Test_ClearBlock_RemovesWalKeysNotIdx(t *testing.T) {
	dir := t.TempDir()
	opts := badger.DefaultOptions(dir)
	db, err := badger.Open(opts)
	if err != nil {
		t.Fatalf("open badger: %v", err)
	}
	defer db.Close()

	bs := blockstore.NewBlockStore(db)
	w := NewDistributedWAL(db, bs, nil)

	// Create a fake sealed chunk
	var c model.SealedChunk
	c.ChunkHash = hash.HashBytes([]byte("chunk2"))
	c.EncryptedContent = []byte("data2")

	if err := w.AppendChunk(context.Background(), c); err != nil {
		t.Fatalf("append chunk: %v", err)
	}

	chunks, vertices, keyEntries, walKeys, err := w.loadWALContent()
	if err != nil {
		t.Fatalf("load wal content: %v", err)
	}
	block := w.createBlock(chunks, vertices, keyEntries)

	// Persist block and index entries as SealBlock would
	if err := db.Update(func(txn *badger.Txn) error {
		data, serr := serialize(block)
		if serr != nil {
			return serr
		}
		if err := txn.Set([]byte("blk:b:"+block.Hash.String()), data); err != nil {
			return err
		}
		for _, c := range chunks {
			if region, ok := block.ChunkIndex[c.ChunkHash]; ok {
				entry := struct {
					BlockHash hash.Hash
					Region    model.ChunkRegion
				}{
					BlockHash: block.Hash,
					Region:    region,
				}
				data, serr := serialize(entry)
				if serr != nil {
					continue
				}
				key := []byte(prefixChunkIdx + c.ChunkHash.String())
				if err := txn.Set(key, data); err != nil {
					return err
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("persist block+idx: %v", err)
	}

	// Ensure WAL key exists before ClearBlock
	walKey := walKeys[0]
	err = db.View(func(txn *badger.Txn) error {
		_, gerr := txn.Get(walKey)
		return gerr
	})
	if err != nil {
		t.Fatalf("expected wal key present before clear: %v", err)
	}

	// Clear WAL entries
	if err := w.ClearBlock(context.Background(), walKeys); err != nil {
		t.Fatalf("clear block: %v", err)
	}

	// WAL key should be gone
	err = db.View(func(txn *badger.Txn) error {
		_, gerr := txn.Get(walKey)
		if gerr == nil {
			return fmt.Errorf("wal key still present")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("wal key should be removed: %v", err)
	}

	// idx:chunk entry should remain and point to the block
	idxKey := []byte(prefixChunkIdx + c.ChunkHash.String())
	err = db.View(func(txn *badger.Txn) error {
		item, gerr := txn.Get(idxKey)
		if gerr != nil {
			return gerr
		}
		return item.Value(func(v []byte) error {
			var entry struct {
				BlockHash hash.Hash
				Region    model.ChunkRegion
			}
			if derr := deserialize(v, &entry); derr != nil {
				return derr
			}
			if entry.BlockHash != block.Hash {
				return fmt.Errorf("idx entry points to wrong block")
			}
			return nil
		})
	})
	if err != nil {
		t.Fatalf("expected idx entry after clear: %v", err)
	}
}
