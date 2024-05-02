package main

import (
	"OuroborosDB/pkg/buzhashChunker"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
)

var (
	// mode 4 works
	mode             = 3     // 1 = NewTransaction, 2 = Update, 3 = NewWriteBatch, 4 = Update without goroutine
	limitConcurrency = 10000 // not used for mode 4
)

func main() {
	db, err := badger.Open(badger.DefaultOptions("./tmp"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	absoluteDataPath := toAbsolutePath("./data/ChunkingChampions/")
	var chunksGlobal []buzhashChunker.ChunkData
	wg := sync.WaitGroup{}
	limitConcurrencyChan := make(chan struct{}, limitConcurrency)

	err = filepath.WalkDir(absoluteDataPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// get bug file
		file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			log.Fatalf("Error opening file: %v", err)
		}

		chunks, err := buzhashChunker.ChunkReader(file)
		if err != nil {
			log.Fatalf("Error chunking data: %v", err)
		}
		chunksGlobal = append(chunksGlobal, chunks...)

		for _, chunk := range chunks {
			chunk := chunk
			wg.Add(1)
			limitConcurrencyChan <- struct{}{}
			if mode == 1 {
				go func() {
					defer wg.Done()

					// Start a writable transaction.
					txn := db.NewTransaction(true)
					defer txn.Discard()

					// Set a key with a value.
					if err := txn.Set(chunk.Hash[:], chunk.Data); err != nil {
						log.Fatal(err)
					}

					// Commit the transaction and check for error.
					if err := txn.Commit(); err != nil {
						log.Fatal(err)

					}

					txn.Commit()
					<-limitConcurrencyChan
				}()
			} else if mode == 2 {
				go func() {
					defer wg.Done()

					err := db.Update(func(txn *badger.Txn) error {
						return txn.Set(chunk.Hash[:], chunk.Data)
					})
					if err != nil {
						log.Fatalf("Error writing chunk: %v", err)
					}
					<-limitConcurrencyChan
				}()
			} else if mode == 3 {
				go func() {
					defer wg.Done()

					wb := db.NewWriteBatch()
					defer wb.Cancel()

					err := wb.Set(chunk.Hash[:], chunk.Data)
					if err != nil {
						log.Fatalf("Error writing chunk: %v", err)
					}

					err = wb.Flush()
					if err != nil {
						log.Fatalf("Error flushing write batch: %v", err)
					}
					<-limitConcurrencyChan
				}()
			} else if mode == 4 {
				err := db.Update(func(txn *badger.Txn) error {
					return txn.Set(chunk.Hash[:], chunk.Data)
				})
				if err != nil {
					log.Fatalf("Error writing chunk: %v", err)
				}
				wg.Done()
				<-limitConcurrencyChan
			}
		}

		return nil
	})
	if err != nil {
		log.Fatalf("Error walking the path: %v", err)
	}

	wg.Wait()
	time.Sleep(5 * time.Second) // give a bit good will

	txn := db.NewTransaction(true)
	defer txn.Discard()
	notFoundCounter := 0

	// Read back the key.
	for _, chunk := range chunksGlobal {
		item, err := txn.Get(chunk.Hash[:])
		if err != nil {
			if err == badger.ErrKeyNotFound {
				notFoundCounter++
				continue
			} else {
				log.Fatalf("Error getting value: %v", err)
			}
		}

		// Extract the value.
		value, err := item.ValueCopy(nil)
		if err != nil {
			log.Fatal(err)
		}

		// compare the value with the original data
		if string(value) != string(chunk.Data) {
			log.Println("Error: The original data and the data from the keyValStore are not the same", len(chunk.Data), "-", len(value))
		}
	}

	percentNotFound := float64(notFoundCounter) / float64(len(chunksGlobal)) * 100

	fmt.Println("not found chunks :", percentNotFound, "%  Chunks:", len(chunksGlobal), "Chunks not found:", notFoundCounter)
}

func toAbsolutePath(relativePathOrAbsolute string) string {
	absolutePath, err := filepath.Abs(relativePathOrAbsolute)
	if err != nil {
		// handle error
		return ""
	}
	return absolutePath
}
