package storageService

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"time"
)

type RootEventsIndex struct {
	Key        []byte
	KeyOfEvent []byte
}

// Same as Event struct in storageService.go but without some unnecessary fields

func (ss *StorageService) CreateRootEvent(title string) (Event, error) {
	// Create a new IndexEvent
	item := Event{
		Key:            []byte{},
		Level:          time.Now().UnixNano(),
		EventHash:      [64]byte{},
		ContentHashes:  [][64]byte{},
		MetadataHashes: [][64]byte{},
	}

	// Check if RootEvent with the same title already exists
	otherRootEvent, err := ss.kv.GetKeysWithPrefix([]byte("RootEvent:" + title + ":"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return Event{}, err
	}

	if len(otherRootEvent) > 0 {
		log.Fatalf("Error creating new root event: RootEvent with the same title already exists")
		return Event{}, fmt.Errorf("Error creating new root event: RootEvent with the same title already exists")
	}

	item.MetadataHashes, err = ss.storeDataInChunkStore([]byte(title))
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return Event{}, err
	}

	item.EventHash = item.CreateDetailsMetaHash()
	item.Key = GenerateKeyFromPrefixAndHash("Event:", item.EventHash)

	//TODO REMOVE
	item.PrettyPrint()

	// Serialize the EventChainItem using gob
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(item); err != nil {
		log.Fatalf("Error encoding item: %v", err)
		return Event{}, err
	}

	// Write the EventChainItem to the keyValStore
	ss.kv.Write(item.Key, buf.Bytes())

	// define the event as entry point for the EventChain
	ss.kv.Write([]byte("RootEvent:"+title+":"+fmt.Sprint(item.Level)), item.Key)

	return item, err
}

func (ss *StorageService) GetAllRootEvents() ([]Event, error) {
	// Get all keys from the keyValStore
	rootIndex, err := ss.GetRootIndex()
	if err != nil {
		log.Fatalf("Error getting root index: %v", err)
		return nil, err
	}

	if len(rootIndex) == 0 {
		return nil, nil
	}

	rootEvents := []Event{}

	for _, indexItem := range rootIndex {
		rootEvent, error := GetEvent(ss.kv, indexItem.KeyOfEvent)
		if error != nil {
			log.Fatalf("Error getting root event: %v", error)
			return nil, error
		}

		rootEvents = append(rootEvents, rootEvent)
	}

	return rootEvents, nil
}

func (ss *StorageService) GetRootIndex() ([]RootEventsIndex, error) {
	// Get all keys from the keyValStore
	rootIndex, err := ss.kv.GetKeysWithPrefix([]byte("RootEvent:"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return nil, err
	}

	if len(rootIndex) == 0 {
		return nil, nil
	}

	revi := []RootEventsIndex{}
	for _, ev := range rootIndex {
		revi = append(revi, RootEventsIndex{Key: ev[0], KeyOfEvent: ev[1]})
	}

	return revi, nil
}

func (ss *StorageService) GetRootEventsWithTitle(title string) ([]Event, error) {
	rootKeys, err := ss.kv.GetKeysWithPrefix([]byte("RootEvent:" + title + ":"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return nil, err
	}

	if len(rootKeys) == 0 {
		return nil, nil
	}

	rootEvents := []Event{}
	for _, key := range rootKeys {
		rootEvent, error := GetEvent(ss.kv, key[1])
		if error != nil {
			log.Fatalf("Error getting root event: %v", error)
			return nil, error
		}

		rootEvents = append(rootEvents, rootEvent)
	}

	return rootEvents, nil
}
