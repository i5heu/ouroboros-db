package keyValStore

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/dgraph-io/badger/v4"
)

type StoreConfig struct {
	Paths            []string // absolute path at the moment only first path is supported
	minimumFreeSpace int      // in GB
}

type KeyValStore struct {
	config   StoreConfig
	badgerDB *badger.DB
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
}

func (k *KeyValStore) startKeyValStore() error {
	opts := badger.DefaultOptions(k.config.Paths[0])
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
	err := k.badgerDB.Update(func(txn *badger.Txn) error {
		err := txn.Set(key, content)
		return err
	})
	if err != nil {
		log.Fatal(err)
	}
	return err
}

func (k *KeyValStore) Read(key []byte) ([]byte, error) {
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
	k.badgerDB.Close()
}

func (k *KeyValStore) GetKeysWithPrefix(prefix []byte) ([][][]byte, error) {
	var keysAndValues [][][]byte
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
