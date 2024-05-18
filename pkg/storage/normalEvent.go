package storage

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"

	"github.com/i5heu/ouroboros-db/internal/binaryCoder"
	"github.com/i5heu/ouroboros-db/pkg/types"
)

const (
	EventPrefix = "Event:"
)

type EventOptions struct {
	EventType      types.EventType
	ParentEvent    types.Hash
	RootEvent      types.Hash
	FastMeta       types.FastMeta
	Metadata       types.ChunkMetaCollection
	Content        types.ChunkMetaCollection
	Temporary      types.Binary
	FullTextSearch types.Binary
}

func (s *Storage) CreateNewEvent(options EventOptions) (types.Event, error) {
	// Create a new Event
	item := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventType: options.EventType,
			FastMeta:  options.FastMeta,
		},
		ParentEvent:    options.ParentEvent,
		RootEvent:      options.RootEvent,
		Metadata:       options.Metadata,
		Content:        options.Content,
		Temporary:      options.Temporary,
		FullTextSearch: options.FullTextSearch,
	}

	// Set Level
	item.Level.SetToNow()

	// Check if the parent event exists
	if item.ParentEvent == (types.Hash{}) {
		log.Fatalf("Error creating new event: Parent event was not defined")
		return types.Event{}, errors.New("Error creating new event: Parent event was not defined")
	}

	// Check if the parent event exists
	parentEvent, err := s.GetEvent(item.ParentEvent)
	if err != nil {
		log.Fatalf("Error creating new event: Parent event does not exist")
		return types.Event{}, errors.New("Error creating new event: Parent event does not exist")
	}

	if !item.Temporary {
		// Check if the parent event is not Temporary
		if parentEvent.Temporary {
			log.Fatalf("Error creating new event: Parent event is Temporary and cannot have non-Temporary children")
			return types.Event{}, errors.New("Error creating new event: Parent event is Temporary and cannot have non-Temporary children")
		}
	}

	// Check if root event is valid
	if item.RootEvent == (types.Hash{}) {
		item.RootEvent = parentEvent.RootEvent
	}
	if item.RootEvent != parentEvent.RootEvent {
		log.Fatalf("Error creating new event: Parent event is not part of the same EventChain")
		return types.Event{}, errors.New("Error creating new event: Parent event is not part of the same EventChain")
	}

	item.EventIdentifier.EventHash = item.CreateDetailsMetaHash()

	// Serialize the Event using Protocol Buffers
	data, err := binaryCoder.EventToByte(item)
	if err != nil {
		log.Fatalf("Error serializing item: %v", err)
		return types.Event{}, err
	}

	// Write the Event to the keyValStore
	err = s.kv.Write(GenerateKeyFromPrefixAndHash("Event:", item.EventIdentifier.EventHash), data)
	if err != nil {
		log.Fatalf("Error writing item: %v", err)
		return types.Event{}, err
	}

	return item, err
}

func (s *Storage) GetEvent(hashOfEvent types.Hash) (types.Event, error) {
	// Read the Event from the keyValStore
	value, err := s.kv.Read(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent))
	if err != nil {
		return types.Event{}, fmt.Errorf("Error reading Event with Key: %x: %v", string(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent)), err)
	}

	// Deserialize the Event using Protocol Buffers
	return binaryCoder.ByteToEvent(value)
}

func (s *Storage) GetAllEvents() ([]types.Event, error) {
	items, err := s.kv.GetItemsWithPrefix([]byte("Event:"))
	if err != nil {
		return nil, err
	}

	var events []types.Event
	for _, item := range items {
		data, err := binaryCoder.ByteToEvent(item[1])
		if err != nil {
			log.Fatalf("Error deserializing item: %v", err)
			return nil, err
		}

		events = append(events, data)
	}

	return events, nil
}

func GetEventHashFromKey(key []byte) types.Hash {
	hash, err := hex.DecodeString(string(key[len(EventPrefix):]))
	if err != nil {
		log.Fatalf("Error decoding Event hash from Key: %x: %v", key, err)
		return types.Hash{}
	}

	var eventHash types.Hash
	copy(eventHash[:], hash)
	return eventHash
}
