package main

import (
	"encoding/hex"
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

func main() {
	db, err := badger.Open(badger.DefaultOptions("./tmp/"))
	if err != nil {
		fmt.Println("Error opening DB: ", err)
		return
	}
	defer db.Close()

	err = db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			err := item.Value(func(val []byte) error {
				fmt.Printf("Key=%s, Value Size=%d bytes\n", hex.EncodeToString(key), len(val))
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error during iteration: ", err)
	}
}
