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
	EventType      types.EventType           // required
	ParentEvent    types.Hash                // required
	RootEvent      types.Hash                // required
	FastMeta       types.FastMeta            // optional
	Metadata       types.ChunkMetaCollection // optional
	Content        types.ChunkMetaCollection // optional
	Temporary      types.Binary              // optional
	FullTextSearch types.Binary              // optional
}

func (s *Storage) CreateNewEvent(options EventOptions) (types.Event, error) {
	// Create a new Event
	item := types.Event{
		EventType:      options.EventType,
		ParentEvent:    options.ParentEvent,
		RootEvent:      options.RootEvent,
		FastMeta:       options.FastMeta,
		Metadata:       options.Metadata,
		Content:        options.Content,
		Temporary:      options.Temporary,
		FullTextSearch: options.FullTextSearch,
	}

	// set Level
	item.Level.SetToNow()

	// check if the parent event exists
	if item.ParentEvent == [64]byte{} {
		log.Fatalf("Error creating new event: Parent event was not defined")
		return types.Event{}, errors.New("Error creating new event: Parent event was not defined")
	}

	// check if the parent event exists
	parentEvent, err := s.GetEvent(item.ParentEvent)
	if err != nil {
		log.Fatalf("Error creating new event: Parent event does not exist")
		return types.Event{}, errors.New("Error creating new event: Parent event does not exist")
	}

	if !item.Temporary {
		// check if the parent event is not Temporary
		if parentEvent.Temporary {
			log.Fatalf("Error creating new event: Parent event is Temporary and can not have non-Temporary children")
			return types.Event{}, errors.New("Error creating new event: Parent event is Temporary and can not have non-Temporary children")
		}
	}

	// check if root event is valid
	if item.RootEvent == [64]byte{} {
		item.RootEvent = parentEvent.RootEvent
	}
	if item.RootEvent != parentEvent.RootEvent {
		log.Fatalf("Error creating new event: Parent event is not part of the same EventChain")
		return types.Event{}, errors.New("Error creating new event: Parent event is not part of the same EventChain")
	}

	item.EventHash = item.CreateDetailsMetaHash()

	// Serialize the EventProto using Protocol Buffers
	data, err := binaryCoder.EventToByte(item)
	if err != nil {
		log.Fatalf("Error serializing item: %v", err)
		return types.Event{}, err
	}

	// Write the EventChainItem to the keyValStore
	err = s.kv.Write(item.Title, data)
	if err != nil {
		log.Fatalf("Error writing item: %v", err)
		return types.Event{}, err
	}

	return item, err
}

func (s *Storage) GetEvent(hashOfEvent [64]byte) (types.Event, error) {
	// Read the EventChainItem from the keyValStore
	value, err := s.kv.Read(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent))
	if err != nil {
		return types.Event{}, fmt.Errorf("Error reading Event with Key: %x: %v", string(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent)), err)
	}

	// Deserialize the EventProto using Protocol Buffers

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

func GetEventHashFromKey(key []byte) [64]byte {
	hash, err := hex.DecodeString(string(key[len(EventPrefix):]))
	if err != nil {
		log.Fatalf("Error decoding Event hash from Key: %x: %v", key, err)
		return [64]byte{}
	}

	var eventHash [64]byte
	copy(eventHash[:], hash)
	return eventHash
}
