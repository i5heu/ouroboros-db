/*
!! Currently the database is in a very early stage of development !!
!! It must not be used in production environments !!
*/
package ouroboros

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dgraph-io/badger/v4"
	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-db/internal/blockstore"
	"github.com/i5heu/ouroboros-db/internal/cas"
	"github.com/i5heu/ouroboros-db/internal/encryption"
	"github.com/i5heu/ouroboros-db/internal/wal"
	"github.com/i5heu/ouroboros-db/pkg/storage"
)

const (
	CurrentDbVersion = "v0.1.1-alpha-3"
)

type OuroborosDB struct {
	log    *slog.Logger
	config Config
	db     *badger.DB
	CAS    storage.CAS
}

func New(conf Config) (*OuroborosDB, error) { // A
	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}

	opts := badger.DefaultOptions(conf.Paths[0])
	// Suppress Badger's default logging to stdout/stderr unless we want it.
	opts.Logger = nil // Disable default badger logger to avoid noise in CLI

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger db: %w", err)
	}

	// Initialize Storage Components
	bs := blockstore.NewBlockStore(db)
	enc := encryption.NewEncryptionService()
	// DistributedWAL requires DataRouter, but we can pass nil for local-only CLI
	w := wal.NewDistributedWAL(db, bs, nil)

	// Load or Generate Identity
	identityPath := filepath.Join(conf.Paths[0], "identity.key")
	pubKey, privKey, err := loadOrGenerateIdentity(identityPath)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load identity: %w", err)
	}

	// Initialize CAS
	c := cas.New(w, bs, enc, pubKey, privKey)

	return &OuroborosDB{
		log:    conf.Logger,
		config: conf,
		db:     db,
		CAS:    c,
	}, nil
}

// Close closes the database and releases resources.
func (db *OuroborosDB) Close() error {
	if db.db != nil {
		return db.db.Close()
	}
	return nil
}

// DBStats contains statistics about the database.
type DBStats struct {
	Vertices    int64
	Chunks      int64
	Blocks      int64
	BlockSlices int64
	Keys        int64
}

// GetStats returns statistics about the database.
func (db *OuroborosDB) GetStats() (DBStats, error) {
	var stats DBStats

	err := db.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			key := string(it.Item().Key())
			switch {
			case len(key) >= 10 && key[:10] == "wal:vertex":
				stats.Vertices++
			case len(key) >= 9 && key[:9] == "wal:chunk":
				stats.Chunks++
			case len(key) >= 7 && key[:7] == "wal:key":
				stats.Keys++
			case len(key) >= 6 && key[:6] == "blk:b:":
				stats.Blocks++
			case len(key) >= 6 && key[:6] == "blk:s:":
				stats.BlockSlices++
			}
		}
		return nil
	})

	return stats, err
}

func loadOrGenerateIdentity(path string) ([]byte, []byte, error) {
	// Try to read existing
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, nil, err
		}
		// Format: [4 byte pub len][pub key][priv key]
		// Actually, let's just serialize simply.
		// Since sizes are fixed (or can be inferred), we can just split?
		// But keys can vary in size (KEM only vs Full).
		// Let's use a simple TLV or length prefix.
		if len(data) < 4 {
			return nil, nil, fmt.Errorf("invalid identity file")
		}
		pubLen := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
		if len(data) < 4+pubLen {
			return nil, nil, fmt.Errorf("invalid identity file structure")
		}
		pubKey := data[4 : 4+pubLen]
		privKey := data[4+pubLen:]
		return pubKey, privKey, nil
	}

	// Generate new
	c := crypt.New()
	pub := c.Keys.GetPublicKey()
	priv := c.Keys.GetPrivateKey()

	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		return nil, nil, err
	}
	pubSign, err := pub.MarshalBinarySign()
	if err != nil {
		return nil, nil, err
	}
	pubBytes := make([]byte, len(pubKEM)+len(pubSign))
	copy(pubBytes, pubKEM)
	copy(pubBytes[len(pubKEM):], pubSign)

	privKEM, err := priv.MarshalBinaryKEM()
	if err != nil {
		return nil, nil, err
	}
	privSign, err := priv.MarshalBinarySign()
	if err != nil {
		return nil, nil, err
	}
	privBytes := make([]byte, len(privKEM)+len(privSign))
	copy(privBytes, privKEM)
	copy(privBytes[len(privKEM):], privSign)

	// Save to disk
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	pubLen := len(pubBytes)
	header := []byte{
		byte(pubLen >> 24),
		byte(pubLen >> 16),
		byte(pubLen >> 8),
		byte(pubLen),
	}
	if _, err := f.Write(header); err != nil {
		return nil, nil, err
	}
	if _, err := f.Write(pubBytes); err != nil {
		return nil, nil, err
	}
	if _, err := f.Write(privBytes); err != nil {
		return nil, nil, err
	}

	return pubBytes, privBytes, nil
}
