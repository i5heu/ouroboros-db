package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"OuroborosDB/pkg/buzhashChunker"

	"github.com/dgraph-io/badger/v4"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: chunkerBenchmark <path of data to be chunked> <path of database>")
		os.Exit(1)
	}

	dataPath := os.Args[1]
	dbPath := os.Args[2]

	createDirIfNotExist(dbPath)
	checkDataPath(dataPath)

	absoluteDataPath, err := filepath.Abs(dataPath)
	if err != nil {
		fmt.Printf("Failed to get absolute path of data directory: %s\n", err)
		os.Exit(1)
	}
	absoluteDBPath, err := filepath.Abs(dbPath)
	if err != nil {
		fmt.Printf("Failed to get absolute path of database directory: %s\n", err)
		os.Exit(1)
	}

	fmt.Println("Files to Chunk:", absoluteDataPath)
	fmt.Println("DB Path:", absoluteDBPath)

	opts := badger.DefaultOptions(absoluteDBPath)
	opts.ValueLogFileSize = 1024 * 1024 * 1 // Set max size of each value log file to 1MB

	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = filepath.WalkDir(absoluteDataPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil // Skip directories
		}
		return processFile(db, path)
	})
	if err != nil {
		fmt.Printf("Failed to process files: %s\n", err)
		os.Exit(1)
	}

	// repeat 10 times in a row

	for i := 0; i < 1; i++ {
		cleanDB(db)
	}

}

func cleanDB(db *badger.DB) (err error) {
	// Trigger a manual compaction
	err = db.Flatten(2) // The parameter is the number of concurrent compactions
	if err != nil {
		fmt.Println("Error during Flatten:", err)
	} else {
		fmt.Println("Compaction completed successfully.")
	}

	// clean badgerDB
	err = db.RunValueLogGC(0.1)
	if err != nil {
		if err != badger.ErrNoRewrite {
			fmt.Printf("Failed to clean badgerDB: %s\n", err)
			os.Exit(1)
		}
	}

	return err
}

func processFile(db *badger.DB, filePath string) error {

	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Failed to open file %s: %s\n", filePath, err)
		return nil
	}
	defer file.Close()

	data, err := buzhashChunker.ChunkReader(file)
	if err != nil {
		fmt.Printf("Failed to chunk file %s: %s\n", filePath, err)
		return nil
	}

	fmt.Println("File:", filePath, "Chunks:", len(data))

	// store the chunks in badgerDB
	for _, chunk := range data {
		err = db.Update(func(txn *badger.Txn) error {
			err := txn.Set(chunk.Hash[:], chunk.Chunk)
			fmt.Print(hex.EncodeToString(chunk.Hash[:]), " ", len(chunk.Chunk), "\n")
			if err != nil {
				return fmt.Errorf("error storing chunk: %w", err)
			}
			return nil
		})
	}

	if err != nil {
		fmt.Printf("Failed to store chunks in badgerDB for file %s: %s\n", filePath, err)
	}
	return nil
}

func createDirIfNotExist(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("Database path does not exist, creating...")
		if err := os.MkdirAll(path, os.ModePerm); err != nil {
			fmt.Printf("Failed to create directory: %s\n", err)
			os.Exit(1)
		}
	}
}

func checkDataPath(path string) {
	info, err := os.Stat(path)
	if err != nil {
		fmt.Printf("Failed to access data path: %s\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Println("Provided data path is not a directory")
		os.Exit(1)
	}

	files, err := os.ReadDir(path)
	if err != nil {
		fmt.Printf("Failed to read directory: %s\n", err)
		os.Exit(1)
	}
	if len(files) == 0 {
		fmt.Println("Data path contains no files")
		os.Exit(1)
	}
}
