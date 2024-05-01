package main

import (
	"OuroborosDB/pkg/keyValStore"
	"OuroborosDB/pkg/storage"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	keyValStore := keyValStore.NewKeyValStore()

	keyValStore.Start([]string{toAbsolutePath("./tmp"), toAbsolutePath("/mnt/volume-nbg1-1/tmp")}, 1)
	defer keyValStore.Close()

	ss := storage.NewStorageService(keyValStore)

	//get all RootEvents with the title "Files", if there are none create one
	rootEvents, err := ss.GetRootEventsWithTitle("Files")
	if err != nil {
		fmt.Println("Error getting list of RootEvents:", err)
		return
	}

	var rootEvent storage.Event

	if len(rootEvents) == 0 {
		rootEvent, err = ss.CreateRootEvent("Files")
		if err != nil {
			fmt.Println("Error creating RootEvent:", err)
			return
		}
	} else {
		rootEvent = rootEvents[0]
	}

	absoluteDataPath := toAbsolutePath("./data/ChunkingChampions/")
	performanceTimer := time.Now()

	var filesNumber int64

	// Use a buffered channel to limit the number of goroutines to 8
	semaphore := make(chan struct{}, 8)
	var wg sync.WaitGroup

	err = filepath.WalkDir(absoluteDataPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		semaphore <- struct{}{} // Acquire a token
		wg.Add(1)

		go func(path string) {
			defer func() {
				<-semaphore // Release the token
				wg.Done()
			}()

			if err := processFile(&ss, rootEvent, path); err != nil {
				fmt.Printf("Error processing file %s: %v\n", path, err)
			}
			atomic.AddInt64(&filesNumber, 1)
		}(path)

		return nil
	})
	if err != nil {
		fmt.Printf("Failed to process files: %s\n", err)
		os.Exit(1)
	}

	wg.Wait() // Wait for all goroutines to finish

	// get the file from the keyValStore
	// _, err = ss.GetFile(eventOfFile)
	// if err != nil {
	// 	fmt.Println("Error getting file:", err)
	// 	return
	// }
	fmt.Printf("Processed %d files in %s\n", filesNumber, time.Since(performanceTimer))
	// store in log
	logg, err := os.OpenFile("log.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("Error opening log file:", err)
		return
	}

	_, err = logg.WriteString(fmt.Sprintf("Processed %d files in %s\n", filesNumber, time.Since(performanceTimer)))
	if err != nil {
		fmt.Println("Error writing to log file:", err)
		return
	}
}

func processFile(ss *storage.Service, rootEvent storage.Event, filePath string) error {
	// open file and read content
	fileInput, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return err
	}

	// store a file in the keyValStore as child of the rootEvent
	_, err = ss.StoreFile(storage.StoreFileOptions{
		EventToAppendTo: rootEvent,
		Metadata:        []byte(filePath),
		File:            fileInput,
		Temporary:       false,
		FullTextSearch:  false,
	})

	if err != nil {
		fmt.Println("Error storing file:", err)
		return err
	}

	return nil
}

func toAbsolutePath(relativePathOrAbsolute string) string {
	absolutePath, err := filepath.Abs(relativePathOrAbsolute)
	if err != nil {
		// handle error
		return ""
	}
	return absolutePath
}
