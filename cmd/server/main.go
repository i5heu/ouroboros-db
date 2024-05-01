package main

import (
	"OuroborosDB/pkg/keyValStore"
	"OuroborosDB/pkg/storageService"
	"fmt"
	"path/filepath"
)

func main() {
	keyValStore := keyValStore.NewKeyValStore()

	keyValStore.Start([]string{toAbsolutePath("./tmp"), toAbsolutePath("/mnt/volume-nbg1-1/tmp")}, 1)
	defer keyValStore.Close()

	//get all RootEvents with the title "Files", if there are none create one
	rootEvents, err := storageService.GetRootEventsWithTitle(*keyValStore, "Files")
	if err != nil {
		fmt.Println("Error getting list of RootEvents:", err)
		return
	}

	var rootEvent storageService.Event

	if len(rootEvents) == 0 {
		rootEvent, err = storageService.CreateRootEvent(*keyValStore, "Files")
		if err != nil {
			fmt.Println("Error creating RootEvent:", err)
			return
		}
	} else {
		rootEvent = rootEvents[0]
	}

	rootEvent.PrettyPrint()

	// store a file in the keyValStore as child of the rootEvent
	eventOfFile, err := storageService.StoreFile(*keyValStore, rootEvent, []byte("metadata"), []byte("file111"))
	if err != nil {
		fmt.Println("Error storing file:", err)
		return
	}

	// get the file from the keyValStore
	file, err := storageService.GetFile(*keyValStore, eventOfFile)
	if err != nil {
		fmt.Println("Error getting file:", err)
		return
	}

	fmt.Println("File:", string(file))
}

func toAbsolutePath(relativePathOrAbsolute string) string {
	absolutePath, err := filepath.Abs(relativePathOrAbsolute)
	if err != nil {
		// handle error
		return ""
	}
	return absolutePath
}
