package storage

import (
	"fmt"
	"log"
	"time"

	"github.com/i5heu/ouroboros-db/internal/binaryCoder"
	"github.com/i5heu/ouroboros-db/pkg/types"
)

const (
	RootEventPrefix = "RootEvent:"
)

func (s *Storage) CreateRootEvent(title string) (types.Event, error) {
	// Create a new Event
	item := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventType: types.Root,
			FastMeta:  types.FastMeta{[]byte(title)},
		},
		Level:          types.Level(time.Now().UnixNano()),
		Metadata:       types.ChunkMetaCollection{},
		Content:        types.ChunkMetaCollection{},
		ParentEvent:    types.Hash{},
		RootEvent:      types.Hash{},
		Temporary:      types.Binary(false),
		FullTextSearch: types.Binary(false),
	}

	// Check if RootEvent with the same title already exists
	otherRootEvent, err := s.kv.GetItemsWithPrefix([]byte("RootEvent:" + title + ":"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return types.Event{}, err
	}

	if len(otherRootEvent) > 0 {
		log.Fatalf("Error creating new root event: RootEvent with the same title already exists")
		return types.Event{}, fmt.Errorf("Error creating new root event: RootEvent with the same title already exists")
	}

	item.Metadata, err = s.storeDataInChunkStore([]byte(title))
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return types.Event{}, err
	}

	item.EventIdentifier.EventHash = item.CreateDetailsMetaHash()
	item.RootEvent = item.EventIdentifier.EventHash

	// Serialize the Event using gob
	data, err := binaryCoder.EventToByte(item)
	if err != nil {
		log.Fatalf("Error serializing Event: %v", err)
		return types.Event{}, err
	}

	// Write the Event to the keyValStore
	s.kv.Write(GenerateKeyFromPrefixAndHash("Event:", item.EventIdentifier.EventHash), data)

	// define the event as entry point for the EventChain
	s.kv.Write([]byte("RootEvent:"+title+":"+fmt.Sprint(item.Level)), GenerateKeyFromPrefixAndHash("Event:", item.EventIdentifier.EventHash))

	return item, err
}

func (s *Storage) GetAllRootEvents() ([]types.Event, error) {
	// Get all keys from the keyValStore
	rootIndex, err := s.GetRootIndex()
	if err != nil {
		log.Fatalf("Error getting root index: %v", err)
		return nil, err
	}

	if len(rootIndex) == 0 {
		return nil, nil
	}

	rootEvents := []types.Event{}

	for _, indexItem := range rootIndex {
		rootEvent, error := s.GetEvent(indexItem.Hash)
		if error != nil {
			log.Fatalf("Error getting root event: %v", error)
			return nil, error
		}

		rootEvents = append(rootEvents, rootEvent)
	}

	return rootEvents, nil
}

func (s *Storage) GetRootIndex() ([]types.RootEventsIndex, error) {
	// Get all keys from the keyValStore
	rootIndex, err := s.kv.GetItemsWithPrefix([]byte("RootEvent:"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return nil, err
	}

	if len(rootIndex) == 0 {
		return nil, nil
	}

	revi := []types.RootEventsIndex{}
	for _, item := range rootIndex {
		revi = append(revi, types.RootEventsIndex{Title: item[0], Hash: GetEventHashFromKey(item[1])})
	}

	return revi, nil
}

func (s *Storage) GetRootEventsWithTitle(title string) ([]types.Event, error) {
	rootIndex, err := s.kv.GetItemsWithPrefix([]byte("RootEvent:" + title + ":"))
	if err != nil {
		log.Fatalf("Error getting keys: %v", err)
		return nil, err
	}

	if len(rootIndex) == 0 {
		return nil, nil
	}

	rootEvents := []types.Event{}
	for _, item := range rootIndex {
		rootEvent, error := s.GetEvent(GetEventHashFromKey(item[1]))
		if error != nil {
			log.Fatalf("Error getting root event: %v", error)
			return nil, error
		}

		rootEvents = append(rootEvents, rootEvent)
	}

	return rootEvents, nil
}
