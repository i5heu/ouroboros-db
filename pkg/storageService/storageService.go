package storageService

import (
	"OuroborosDB/pkg/buzhashChunker"
	"OuroborosDB/pkg/keyValStore"
	"bytes"
	"crypto/sha512"
	"encoding/gob"
	"log"
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

func CreateNewEventChain(kv keyValStore.KeyValStore, eventChainTitle string, contentHashes [][64]byte) {
	// Create a new EventChainItem
	item := EventChainItem{
		Level:             time.Now().UnixNano(),
		ContentMetaHash:   [64]byte{},
		ContentHashes:     contentHashes,
		MetadataHashes:    StoreDataInChunkStore(kv, []byte(eventChainTitle)),
		HashOfParentEvent: [64]byte{},
		HashOfSourceEvent: [64]byte{},
		Temporary:         false,
	}

	item.ContentMetaHash = createContentMetaHash(item)

	PrettyPrintEventChainItem(item)

	// Serialize the EventChainItem using gob
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(item); err != nil {
		log.Fatalf("Error encoding item: %v", err)
	}

	// Write the EventChainItem to the keyValStore
	key := append([]byte("EventChainItem:"), item.ContentMetaHash[:]...)
	kv.Write(key, buf.Bytes())
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

func StoreDataInChunkStore(kv keyValStore.KeyValStore, data []byte) [][64]byte {
	chunks, err := buzhashChunker.ChunkBytes(data)
	if err != nil {
		log.Fatalf("Error chunking data: %v", err)
		return nil
	}

	keys := make([][64]byte, 0)

	for _, chunk := range chunks {
		kv.Write(chunk.Hash[:], chunk.Chunk)
	}

	return keys
}
