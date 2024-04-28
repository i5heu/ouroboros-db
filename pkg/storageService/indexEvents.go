package storageService

import (
	"OuroborosDB/pkg/keyValStore"
	"bytes"
	"encoding/gob"
	"log"
)

type EventChains struct {
	Key   []byte
	Title string
	Level int64
}

func GetListOfIndexEvents(kv keyValStore.KeyValStore) []EventChains {

	indexEvents := []EventChains{}

	// Get all keys from the keyValStore
	keys := kv.GetKeysWithPrefix([]byte("TopEventChainItem:"))

	// Iterate over all keys and print the keys
	for _, key := range keys {
		// Read the EventChainItem from the keyValStore
		value, err := kv.Read(key)
		if err != nil {
			log.Fatalf("Error reading key: %v", err)
		}

		// Deserialize the EventChainItem using gob
		item := EventChainItem{}
		dec := gob.NewDecoder(bytes.NewReader(value))
		if err := dec.Decode(&item); err != nil {
			log.Fatalf("Error decoding item: %v", err)
		}

		metadata := ""

		for _, hash := range item.MetadataHashes {
			value, err := kv.Read(hash[:])
			if err != nil {
				log.Fatalf("Error reading key: %v", err)
				return nil
			}

			metadata += string(value)
		}

		indexEvents = append(indexEvents, EventChains{Key: key, Title: metadata, Level: item.Level})
	}

	return indexEvents
}
