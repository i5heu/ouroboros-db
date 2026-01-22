package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db"
)

func main() {
	storeCmd := flag.NewFlagSet("store", flag.ExitOnError)
	retrieveCmd := flag.NewFlagSet("retrieve", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("Usage: ouroboros-cli <command> [arguments]")
		fmt.Println("Commands:")
		fmt.Println("  store <file>")
		fmt.Println("  retrieve <vertex-hash> <output-file>")
		fmt.Println("  info")
		fmt.Println("  flush")
		os.Exit(1)
	}

	dataDir := getDataDir()
	conf := ouroboros.Config{
		Paths: []string{dataDir},
	}

	dbInstance, err := ouroboros.New(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing DB: %v\n", err)
		os.Exit(1)
	}
	defer dbInstance.Close()

	switch os.Args[1] {
	case "flush":
		fmt.Println("Flushing WAL...")
		if err := dbInstance.CAS.Flush(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Flush successful.")
	case "info":
		stats, err := dbInstance.GetStats()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting stats: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Database Statistics:")
		fmt.Printf("  Vertices:     %d\n", stats.Vertices)
		fmt.Printf("  Chunks:       %d\n", stats.Chunks)
		fmt.Printf("  Blocks:       %d\n", stats.Blocks)
		fmt.Printf("  BlockSlices:  %d\n", stats.BlockSlices)
		fmt.Printf("  Keys:         %d\n", stats.Keys)

	case "store":
		storeCmd.Parse(os.Args[2:])
		if storeCmd.NArg() < 1 {
			fmt.Println("Usage: ouroboros-cli store <file>")
			os.Exit(1)
		}
		filePath := storeCmd.Arg(0)
		storeFile(dbInstance, filePath)

	case "retrieve":
		retrieveCmd.Parse(os.Args[2:])
		if retrieveCmd.NArg() < 2 {
			fmt.Println("Usage: ouroboros-cli retrieve <vertex-hash> <output-file>")
			os.Exit(1)
		}
		hashStr := retrieveCmd.Arg(0)
		outPath := retrieveCmd.Arg(1)
		retrieveFile(dbInstance, hashStr, outPath)

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func getDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	dir := filepath.Join(home, ".ouroboros", "data")
	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(err)
	}
	return dir
}

func storeFile(db *ouroboros.OuroborosDB, filePath string) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	vertex, err := db.CAS.StoreContent(context.Background(), content, hash.Hash{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error storing content: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Stored successfully. Vertex Hash: %s\n", vertex.Hash)
}

func retrieveFile(db *ouroboros.OuroborosDB, hashStr string, outPath string) {
	h, err := parseHash(hashStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid hash: %v\n", err)
		os.Exit(1)
	}

	content, err := db.CAS.GetContent(context.Background(), h)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error retrieving content: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, content, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Retrieved successfully.")
}

func parseHash(s string) (hash.Hash, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return hash.Hash{}, err
	}
	if len(b) != 64 {
		return hash.Hash{}, fmt.Errorf("invalid hash length: expected 64 bytes, got %d", len(b))
	}
	var h hash.Hash
	copy(h[:], b)
	return h, nil
}
