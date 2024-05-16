package storage

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/i5heu/ouroboros-db/internal/binaryCoder"
	"github.com/i5heu/ouroboros-db/pkg/types"
)

const (
	EventPrefix = "Event:"
)

type EventOptions struct {
	HashOfParentEvent [64]byte
	ContentHashes     [][64]byte // optional
	MetadataHashes    [][64]byte // optional
	Temporary         bool       // optional
	FullTextSearch    bool       // optional
}

func (ss *storage) CreateNewEvent(options EventOptions) (types.Event, error) {
	// Create a new Event
	item := types.Event{
		Key:               []byte{},
		EventHash:         [64]byte{},
		Level:             time.Now().UnixNano(),
		ContentHashes:     options.ContentHashes,
		MetadataHashes:    options.MetadataHashes,
		HashOfParentEvent: options.HashOfParentEvent,
		HashOfRootEvent:   [64]byte{},
		Temporary:         false,
		FullTextSearch:    false,
	}

	// check if the parent event exists
	if item.HashOfParentEvent == [64]byte{} {
		log.Fatalf("Error creating new event: Parent event was not defined")
		return types.Event{}, errors.New("Error creating new event: Parent event was not defined")
	}

	// check if the parent event exists
	parentEvent, err := ss.GetEvent(item.HashOfParentEvent)
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
	if item.HashOfRootEvent == [64]byte{} {
		item.HashOfRootEvent = parentEvent.HashOfRootEvent
	}
	if item.HashOfRootEvent != parentEvent.HashOfRootEvent {
		log.Fatalf("Error creating new event: Parent event is not part of the same EventChain")
		return types.Event{}, errors.New("Error creating new event: Parent event is not part of the same EventChain")
	}

	item.EventHash = item.CreateDetailsMetaHash()
	item.Key = GenerateKeyFromPrefixAndHash("Event:", item.EventHash)

	// Serialize the EventProto using Protocol Buffers
	data, err := binaryCoder.EventToByte(item)
	if err != nil {
		log.Fatalf("Error serializing item: %v", err)
		return types.Event{}, err
	}

	// Write the EventChainItem to the keyValStore
	err = ss.kv.Write(item.Key, data)
	if err != nil {
		log.Fatalf("Error writing item: %v", err)
		return types.Event{}, err
	}

	return item, err
}

func (ss *storage) GetEvent(hashOfEvent [64]byte) (types.Event, error) {
	// Read the EventChainItem from the keyValStore
	value, err := ss.kv.Read(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent))
	if err != nil {
		return types.Event{}, fmt.Errorf("Error reading Event with Key: %x: %v", string(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent)), err)
	}

	// Deserialize the EventProto using Protocol Buffers

	return binaryCoder.ByteToEvent(value)
}

func (ss *storage) GetAllEvents() ([]types.Event, error) {
	items, err := ss.kv.GetItemsWithPrefix([]byte("Event:"))
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
