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

	//Create Event and safe File into Event
	storageService.CreateNewEventChain(*keyValStore, "IndexEvent", [][64]byte{})

	indexItems := storageService.GetListOfIndexEvents(*keyValStore)
	for _, item := range indexItems {
		fmt.Printf("Key: %v, Title: %v, Level: %v\n", item.Key, item.Title, item.Level)
	}
}

func toAbsolutePath(relativePathOrAbsolute string) string {
	absolutePath, err := filepath.Abs(relativePathOrAbsolute)
	if err != nil {
		// handle error
		return ""
	}
	return absolutePath
}
