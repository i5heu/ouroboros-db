package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/pkg/storage"
)

func main() {
	fmt.Println("Starting OuroborosDB example")

	absPath, _ := filepath.Abs("ExamplePath/" + time.Now().String())

	// Initialize OuroborosDB with basic configuration
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{absPath}, // Directory for data storage
		MinimumFreeGB:             1,                 // Minimum free space in GB
		GarbageCollectionInterval: 10,                // GC interval in seconds
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to initialize OuroborosDB: %s", err))
	}
	defer ou.Close()

	// Ensure index is rebuilt for data integrity
	if _, err := ou.Index.RebuildIndex(); err != nil {
		log.Fatal(fmt.Sprintf("Error rebuilding index: %s", err))
	}

	uniqueNameBypass := time.Now().String()

	// Create a root event titled "ExampleRoot"
	rootEvent, err := ou.DB.CreateRootEvent("ExampleRoot" + uniqueNameBypass)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating root event: %s", err))
	}
	fmt.Printf("Created root event")

	// Create a child event under the "ExampleRoot" event
	_, err = ou.DB.CreateNewEvent(storage.EventOptions{
		ParentEvent: rootEvent.EventHash,
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating child event: %s", err))
	}
	fmt.Printf("Created child event\n")

	// Rebuild index after creating the events
	if _, err := ou.Index.RebuildIndex(); err != nil {
		log.Fatal(fmt.Sprintf("Error rebuilding index: %s", err))
	}

	// Retrieve root events with the title "ExampleRoot"
	rootEvents, err := ou.DB.GetRootEventsWithTitle("ExampleRoot" + uniqueNameBypass)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error retrieving root events: %s", err))
	}
	for range rootEvents {
		fmt.Printf("Retrieved root event\n")
	}

	// Retrieve direct children of the "ExampleRoot" event
	children, err := ou.Index.GetDirectChildrenOfEvent(rootEvent.EventHash)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error retrieving children of root event: %s", err))
	}
	for range children {
		fmt.Printf("Retrieved child event\n")
	}

	count := 0
	// create 100 new files
	for i := 0; i < 10000; i++ {
		if count >= 100 {
			fmt.Println("Created", i, "files")
			count = 0
		}
		count++
		_, err = ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: rootEvent,
			File:            generateTestData(10 * 1024 * 1024),
		})
		if err != nil {
			log.Fatal(fmt.Sprintf("Error creating child event: %s", err))
		}
	}
}

func generateText() string {
	var result strings.Builder
	text := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. "
	textLength := len(text)

	// Calculate how many times to repeat the string to reach approximately 1KB
	for i := 0; i < 1024/textLength; i++ {
		result.WriteString(text)
	}

	// If there's any remaining space to fill to make it exactly 1KB
	if remainder := 1024 % textLength; remainder != 0 {
		result.WriteString(text[:remainder])
	}

	return result.String()
}

func generateTestData(size int) []byte {
	// Create a slice of the desired size
	data := make([]byte, size)

	// Fill the slice with random data
	rand.Read(data)

	return data
}
