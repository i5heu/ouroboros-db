package keyValStore

import (
	"encoding/hex"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/i5heu/ouroboros-db/pkg/types"

	"github.com/dgraph-io/badger/v4"
	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

type StoreConfig struct {
	Paths            []string // absolute path at the moment only first path is supported
	MinimumFreeSpace int      // in GB
	Logger           *logrus.Logger
}

type KeyValStore struct {
	config       StoreConfig
	badgerDB     *badger.DB
	readCounter  uint64
	writeCounter uint64
}

func NewKeyValStore(config StoreConfig) (*KeyValStore, error) {
	if config.Logger == nil {
		config.Logger = logrus.New()
	}

	log = config.Logger

	err := config.checkConfig()
	if err != nil {
		return nil, fmt.Errorf("error checking config for KeyValStore: %w", err)
	}

	opts := badger.DefaultOptions(config.Paths[0])
	opts.Logger = nil
	opts.ValueLogFileSize = 1024 * 1024 * 100 // Set max size of each value log file to 100MB
	opts.SyncWrites = false

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	err = displayDiskUsage(config.Paths)
	if err != nil {
		log.Fatal(err)
		return nil, err
	}

	return &KeyValStore{
		config:   config,
		badgerDB: db,
	}, nil
}

func (k *KeyValStore) StartTransactionCounter(paths []string, minimumFreeSpace int) {

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

	// go func() {
	// 	ticker := time.NewTicker(1 * time.Minute)
	// 	defer ticker.Stop()
	// 	for range ticker.C {
	// 		k.Clean()
	// 	}
	// }()

}

func (k *KeyValStore) Write(key []byte, content []byte) error {
	atomic.AddUint64(&k.writeCounter, 1)

	err := k.badgerDB.Update(func(txn *badger.Txn) error {
		return txn.Set(key, content)
	})
	if err != nil {
		log.Fatal(err)
		return err
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

func (k *KeyValStore) BatchWriteNonExistingChunks(chunks types.ChunkCollection) error {
	var keys [][]byte
	for _, chunk := range chunks {
		chunk := chunk
		keys = append(keys, chunk.Hash[:])
	}

	existsMap, err := k.BatchCheckKeyExistence(keys)
	if err != nil {
		log.Fatal("Error checking key existence: ", err)
		return err
	}

	var nonExistingChunks types.ChunkCollection
	for _, chunk := range chunks {
		chunk := chunk
		if !existsMap[string(chunk.Hash[:])] {
			nonExistingChunks = append(nonExistingChunks, chunk)
		}
	}

	err = k.BatchWriteChunk(nonExistingChunks)
	if err != nil {
		log.Fatal("Error writing non-existing chunks: ", err)
		return err
	}

	return nil
}

func (k *KeyValStore) BatchCheckKeyExistence(keys [][]byte) (map[string]bool, error) {
	existsMap := make(map[string]bool)

	err := k.badgerDB.View(func(txn *badger.Txn) error {
		for _, key := range keys {
			key := key
			atomic.AddUint64(&k.readCounter, 1)
			_, err := txn.Get(key)
			if err != nil {
				if err == badger.ErrKeyNotFound {
					existsMap[string(key[:])] = false
				} else {
					return err // return an error for issues other than "key not found"
				}
			} else {
				existsMap[string(key[:])] = true
			}
		}
		return nil
	})

	return existsMap, err
}

func (k *KeyValStore) BatchWriteChunk(chunks types.ChunkCollection) error {

	wb := k.badgerDB.NewWriteBatch()
	defer wb.Cancel()

	for _, chunk := range chunks {
		chunk := chunk

		atomic.AddUint64(&k.writeCounter, 1)
		err := wb.Set(chunk.Hash[:], chunk.Data)
		if err != nil {
			log.Fatal("Error writing chunk: ", err)
			return err
		}
	}

	return wb.Flush()
}

func (k *KeyValStore) verifyChunksExistence(chunks types.ChunkCollection) error {
	var keys [][]byte
	for _, chunk := range chunks {
		keys = append(keys, chunk.Hash[:])
	}

	chunkState, err := k.BatchCheckKeyExistence(keys)
	if err != nil {
		return fmt.Errorf("error checking key existence: %w", err)
	}

	var nonExistingChunks types.ChunkCollection
	for _, chunk := range chunks {
		if !chunkState[string(chunk.Hash[:])] {
			nonExistingChunks = append(nonExistingChunks, chunk)
			fmt.Printf("Missing chunk: %x, chunk data length: %v\n", chunk.Hash, len(chunk.Data))
		} else {
			fmt.Printf("Chunk Found: %x, chunk data length: %v\n", chunk.Hash, len(chunk.Data))
		}
	}

	if len(nonExistingChunks) > 0 {
		return fmt.Errorf("error: non-existing chunks after write: %d", len(nonExistingChunks))
	}

	return nil
}

func (k *KeyValStore) Read(key []byte) ([]byte, error) {
	atomic.AddUint64(&k.readCounter, 1)
	var value []byte
	err := k.badgerDB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("Error reading key %s: %v", hex.EncodeToString(key[:]), err)
	}
	return value, err
}

func (k *KeyValStore) Close() {
	k.Clean()
	k.badgerDB.Close()
}

func (k *KeyValStore) Clean() error {
	err := k.badgerDB.Sync()
	if err != nil {
		return fmt.Errorf("error syncing db: %w", err)
	}

	// flatten the db
	err = k.badgerDB.Flatten(runtime.NumCPU()) // The parameter is the number of concurrent compactions
	if err != nil {
		return fmt.Errorf("error flattening db: %w", err)
	} else {
		log.Info("DB Flattened")
	}

	// clean badgerDB
	err = k.badgerDB.RunValueLogGC(0.1)
	if err != nil {
		if err != badger.ErrNoRewrite {
			return fmt.Errorf("error cleaning db: %w", err)
		}
	}

	return nil
}

// will return all keys and values with the given prefix
func (k *KeyValStore) GetItemsWithPrefix(prefix []byte) ([][][]byte, error) {
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

// func (k *KeyValStore) GetItemsWithPrefixStream(prefix []byte) ([][][]byte, error) {

// 	var keysAndValues [][][]byte
// 	var mu sync.Mutex
// 	wg := sync.WaitGroup{}
// 	atomic.AddUint64(&k.readCounter, 1)

// 	stream := k.badgerDB.NewStream()
// 	stream.NumGo = runtime.NumCPU()
// 	stream.Prefix = prefix
// 	stream.LogPrefix = "Badger.Streaming"

// 	// Process each buffer of key-value pairs
// 	stream.Send = func(buf *z.Buffer) error {
// 		kvList, err := badger.BufferToKVList(buf)
// 		if err != nil {
// 			return err
// 		}

// 		for _, kv := range kvList.GetKv() {
// 			mu.Lock()
// 			keysAndValues = append(keysAndValues, [][]byte{kv.Value, kv.Value})
// 			mu.Unlock()
// 		}

// 		return nil
// 	}

// 	err := stream.Orchestrate(context.Background())
// 	if err != nil {
// 		fmt.Println("Error streaming: ", err)
// 		return nil, err
// 	}

// 	wg.Wait()

// 	return keysAndValues, nil
// }
