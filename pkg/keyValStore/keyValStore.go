package keyValStore

import (
	"OuroborosDB/pkg/buzhashChunker"
	"encoding/hex"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type StoreConfig struct {
	Paths            []string // absolute path at the moment only first path is supported
	minimumFreeSpace int      // in GB
}

type KeyValStore struct {
	config       StoreConfig
	badgerDB     *badger.DB
	readCounter  uint64
	writeCounter uint64
}

func NewKeyValStore() *KeyValStore {
	return &KeyValStore{}
}

func (k *KeyValStore) Start(paths []string, minimumFreeSpace int) {
	k.config = StoreConfig{
		Paths:            paths,
		minimumFreeSpace: minimumFreeSpace,
	}

	err := k.checkConfig()
	if err != nil {
		log.Fatal(err)
	}

	// print the space left and allocated from the db
	fmt.Println("####################")
	displayDiskUsage(k.config.Paths)
	fmt.Println("####################")

	// start the key value store
	k.startKeyValStore()

	// Start the ticker to log operations per second
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			readOps := atomic.SwapUint64(&k.readCounter, 0)
			writeOps := atomic.SwapUint64(&k.writeCounter, 0)
			fmt.Printf("Chunk Read operations/sec: %d, Chunk Write operations/sec: %d\n", readOps, writeOps)
		}
	}()

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			k.Clean()
		}
	}()

}

func (k *KeyValStore) startKeyValStore() error {
	opts := badger.DefaultOptions(k.config.Paths[0])
	opts.Logger = nil
	opts.ValueLogFileSize = 1024 * 1024 * 10 // Set max size of each value log file to 10MB

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
		return err
	}

	k.badgerDB = db
	return nil
}

func (k *KeyValStore) Write(key []byte, content []byte) error {
	atomic.AddUint64(&k.writeCounter, 1)
	err := k.badgerDB.Update(func(txn *badger.Txn) error {
		return txn.Set(key, content)
	})
	if err != nil {
		log.Fatal(err)
	}
	return err
}

func (k *KeyValStore) WriteBatch(batch [][2][]byte) error {
	err := k.badgerDB.Update(func(txn *badger.Txn) error {
		for _, kv := range batch {
			atomic.AddUint64(&k.writeCounter, 1)
			err := txn.Set(kv[0], kv[1])
			if err != nil {
				log.Fatal("Error writing batch: ", err)
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	return err
}

func (k *KeyValStore) WriteBatchChunk(chunks []buzhashChunker.ChunkData) error {

	wb := k.badgerDB.NewWriteBatch()
	defer wb.Cancel()

	for _, chunk := range chunks {
		atomic.AddUint64(&k.writeCounter, 1)
		err := wb.Set(chunk.Hash[:], chunk.Data)
		if err != nil {
			log.Fatal("Error writing chunk: ", err)
			return err
		}
	}

	return wb.Flush()
}

func (k *KeyValStore) Read(key []byte) ([]byte, error) {
	atomic.AddUint64(&k.readCounter, 1)
	var value []byte
	err := k.badgerDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		err = item.Value(func(val []byte) error {
			value = append([]byte{}, val...)
			return nil
		})
		return err
	})
	if err != nil {
		log.Fatal(err, " ", hex.EncodeToString(key))
	}
	return value, err
}

func (k *KeyValStore) Close() {
	k.Clean()
	k.badgerDB.Close()
}

func (k *KeyValStore) Clean() {
	err := k.badgerDB.Flatten(2) // The parameter is the number of concurrent compactions
	if err != nil {
		fmt.Println("Error during Flatten:", err)
	} else {
		fmt.Println("Compaction completed successfully.")
	}

	// clean badgerDB
	err = k.badgerDB.RunValueLogGC(0.1)
	if err != nil {
		if err != badger.ErrNoRewrite {
			fmt.Printf("Failed to clean badgerDB: %s\n", err)
		}
	}
}

func (k *KeyValStore) GetKeysWithPrefix(prefix []byte) ([][][]byte, error) {
	var keysAndValues [][][]byte
	atomic.AddUint64(&k.readCounter, 1)
	err := k.badgerDB.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			k := item.KeyCopy(nil)
			v, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			keysAndValues = append(keysAndValues, [][]byte{k, v})
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
		return nil, err
	}
	return keysAndValues, nil
}
