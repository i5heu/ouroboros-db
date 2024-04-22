package main

import (
	"encoding/hex"
	"fmt"
	"log"

	"github.com/dgraph-io/badger/v4"
)

func main() {
	db, err := badger.Open(badger.DefaultOptions("./tmp"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count int

	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			fmt.Printf("Key: %s\n", hex.EncodeToString(key))
			count++
		}
		return nil
	})

	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Total number of keys: %d\n", count)
}
