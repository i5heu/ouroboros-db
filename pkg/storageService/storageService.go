package storageService

import (
	"OuroborosDB/pkg/buzhashChunker"
	"OuroborosDB/pkg/keyValStore"
	"bytes"
	"crypto/sha512"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

type EventChainItem struct {
	Key               []byte     // this:is:path:[DetailsMetaHash]
	DetailsMetaHash   [64]byte   // SHA-512 hash of all ContentHashes, MetadataHashes, Level, HashOfParentEvent and HashOfSourceEvent and Temporary ("true" or "false" string)
	Level             int64      // unix timestamp of creation time
	ContentHashes     [][64]byte // all hashes in a certain order that allow to reconstruct the content from the values in badgerDB
	MetadataHashes    [][64]byte // If no metadata is present the metadata of the the next previous event with metadata is used
	HashOfParentEvent [64]byte   // the event this item is the child of
	HashOfSourceEvent [64]byte   // the first event that marks a now EventChain
	Temporary         bool       // no non-Temporary event can have a Temporary event as Parent, Temporary events will be removed after some conditions are met, if one deletes a Temporary event all its children will be deleted too
}

func CreateNewIndexEvent(kv keyValStore.KeyValStore, title string) error {
	// Create a new IndexEvent
	item := EventChainItem{
		Level:             time.Now().UnixNano(),
		DetailsMetaHash:   [64]byte{},
		ContentHashes:     [][64]byte{},
		MetadataHashes:    [][64]byte{},
		HashOfParentEvent: [64]byte{},
		HashOfSourceEvent: [64]byte{},
		Temporary:         false,
	}

	var err error
	item.MetadataHashes, err = StoreDataInChunkStore(kv, []byte(title))
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return err
	}

	item.DetailsMetaHash = createContentMetaHash(item)
	item.Key = GenerateKeyFromPrefixAndHash("IndexTopEventChainItem:", item.DetailsMetaHash)

	//TODO REMOVE
	PrettyPrintEventChainItem(item)

	// Serialize the EventChainItem using gob
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(item); err != nil {
		log.Fatalf("Error encoding item: %v", err)
	}

	// Write the EventChainItem to the keyValStore
	kv.Write(item.Key, buf.Bytes())

	return err
}

func CreateNewEventChain(kv keyValStore.KeyValStore, eventChainTitle string, contentHashes [][64]byte) error {
	// Create a new EventChainItem
	item := EventChainItem{
		Key:               []byte{},
		DetailsMetaHash:   [64]byte{},
		Level:             time.Now().UnixNano(),
		ContentHashes:     contentHashes,
		MetadataHashes:    [][64]byte{},
		HashOfParentEvent: [64]byte{},
		HashOfSourceEvent: [64]byte{},
		Temporary:         false,
	}

	var err error
	item.MetadataHashes, err = StoreDataInChunkStore(kv, []byte(eventChainTitle))
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return err
	}

	item.DetailsMetaHash = createContentMetaHash(item)
	item.Key = GenerateKeyFromPrefixAndHash("TopEventChainItem:", item.DetailsMetaHash)

	PrettyPrintEventChainItem(item)

	// Serialize the EventChainItem using gob
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(item); err != nil {
		log.Fatalf("Error encoding item: %v", err)
	}

	// Write the EventChainItem to the keyValStore

	fmt.Println("111 Key: ", item.Key)
	fmt.Println("222 Key: ", string(item.Key))
	fmt.Println("333 Key: ", hex.EncodeToString(item.Key))

	err = kv.Write(item.Key, buf.Bytes())
	if err != nil {
		log.Fatalf("Error writing item: %v", err)
	}

	return err
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

func StoreDataInChunkStore(kv keyValStore.KeyValStore, data []byte) ([][64]byte, error) {
	chunks, err := buzhashChunker.ChunkBytes(data)
	if err != nil {
		log.Fatalf("Error chunking data: %v", err)
		return nil, err
	}

	keys := make([][64]byte, 0)

	for _, chunk := range chunks {
		keys = append(keys, chunk.Hash)
		err := kv.Write(chunk.Hash[:], chunk.Chunk)
		if err != nil {
			log.Fatalf("Error writing chunk: %v", err)
			return nil, err
		}
	}

	return keys, nil
}

func GenerateKeyFromPrefixAndHash(prefix string, hash [64]byte) []byte {
	return append([]byte(prefix), []byte(hex.EncodeToString(hash[:]))...)
}
