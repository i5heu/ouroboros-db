package storageService

import (
	"crypto/sha512"
	"fmt"
	"time"
)

type EventChainItem struct {
	Level             int64      // unix timestamp of creation time
	ContentMetaHash   [64]byte   // SHA-512 hash of all ContentHashes, MetadataHashes, Level, HashOfParentEvent and HashOfSourceEvent and Temporary ("true" or "false" string)
	ContentHashes     [][64]byte // all hashes in a certain order that allow to reconstruct the content from the values in badgerDB
	MetadataHashes    [][64]byte // If no metadata is present the metadata of the the next previous event with metadata is used
	HashOfParentEvent [64]byte
	HashOfSourceEvent [64]byte // the first event that marks a now EventChain
	Temporary         bool     // no non-Temporary event can have a Temporary event as Parent, Temporary events will be removed after some conditions are met, if one deletes a Temporary event all its children will be deleted too
}

func CreateNewEventChain(metaData [][64]byte, contentHashes [][64]byte) {
	// Create a new EventChainItem
	item := EventChainItem{
		Level:             time.Now().UnixNano(),
		ContentMetaHash:   [64]byte{},
		ContentHashes:     contentHashes,
		MetadataHashes:    metaData,
		HashOfParentEvent: [64]byte{},
		HashOfSourceEvent: sha512.Sum512([]byte("StartEventChain")),
		Temporary:         false,
	}

	item.ContentMetaHash = createContentMetaHash(item)

	PrettyPrintEventChainItem(item)

	// Add to Key-Value store
}

func createContentMetaHash(item EventChainItem) [64]byte {
	// create a buffer, put all bytes into the buffer and hash it
	buffer := make([]byte, 0)

	buffer = append(buffer, byte(item.Level))
	for _, hash := range item.ContentHashes {
		buffer = append(buffer, hash[:]...)
	}
	for _, hash := range item.MetadataHashes {
		buffer = append(buffer, hash[:]...)
	}
	buffer = append(buffer, item.HashOfParentEvent[:]...)
	buffer = append(buffer, item.HashOfSourceEvent[:]...)
	if item.Temporary {
		buffer = append(buffer, []byte("true")...)
	} else {
		buffer = append(buffer, []byte("false")...)
	}

	return sha512.Sum512(buffer)
}

func PrettyPrintEventChainItem(item EventChainItem) {
	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		fmt.Println("Error marshalling EventChainItem to JSON:", err)
		return
	}
	fmt.Printf("%d\n", string(jsonBytes))
}
