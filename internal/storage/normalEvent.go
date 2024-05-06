package storage

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	EventPrefix = "Event:"
)

// Event represents an event in the EventChain, the absolute top of a EventChain is a RootEvent, look at rootEvents.go
type Event struct {
	Key               []byte     // type:title:[DetailsMetaHash] title is only used for RootEvents, DetailsMetaHash is the hash of all ContentHashes, MetadataHashes, Level, HashOfParentEvent and HashOfSourceEvent and Temporary ("true" or "false" string)
	EventHash         [64]byte   // SHA-512 hash of all ContentHashes, MetadataHashes, Level, HashOfParentEvent and HashOfSourceEvent and Temporary ("true" or "false" string)
	Level             int64      // unix timestamp of creation time
	ContentHashes     [][64]byte // all hashes in a certain order that allow to reconstruct the content from the values in badgerDB
	MetadataHashes    [][64]byte // If no metadata is present the metadata of the the next previous event with metadata is used
	HashOfParentEvent [64]byte   // the event this item is the child of
	HashOfRootEvent   [64]byte   // the first event that marks a now EventChain
	Temporary         bool       // no non-Temporary event can have a Temporary event as Parent, Temporary events will be removed after some conditions are met, if one deletes a Temporary event all its children will be deleted too
	FullTextSearch    bool       // if true the ContentHashes and MetadataHashes of the event will be used for full-text search, Important: this only applies to the event itself, not to its children
}

type EventOptions struct {
	HashOfParentEvent [64]byte
	ContentHashes     [][64]byte // optional
	MetadataHashes    [][64]byte // optional
	Temporary         bool       // optional
	FullTextSearch    bool       // optional
}

func (ss *Storage) CreateNewEvent(options EventOptions) (Event, error) {
	// Create a new Event
	item := Event{
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
		return Event{}, errors.New("Error creating new event: Parent event was not defined")
	}

	// check if the parent event exists
	parentEvent, err := ss.GetEvent(item.HashOfParentEvent)
	if err != nil {
		log.Fatalf("Error creating new event: Parent event does not exist")
		return Event{}, errors.New("Error creating new event: Parent event does not exist")
	}

	if !item.Temporary {
		// check if the parent event is not Temporary
		if parentEvent.Temporary {
			log.Fatalf("Error creating new event: Parent event is Temporary and can not have non-Temporary children")
			return Event{}, errors.New("Error creating new event: Parent event is Temporary and can not have non-Temporary children")
		}
	}

	// check if root event is valid
	if item.HashOfRootEvent == [64]byte{} {
		item.HashOfRootEvent = parentEvent.HashOfRootEvent
	}
	if item.HashOfRootEvent != parentEvent.HashOfRootEvent {
		log.Fatalf("Error creating new event: Parent event is not part of the same EventChain")
		return Event{}, errors.New("Error creating new event: Parent event is not part of the same EventChain")
	}

	item.EventHash = item.CreateDetailsMetaHash()
	item.Key = GenerateKeyFromPrefixAndHash("Event:", item.EventHash)

	// Serialize the EventChainItem using
	pbEvent := convertToProtoEvent(item)
	data, err := proto.Marshal(pbEvent)
	if err != nil {
		log.Fatalf("Error encoding item: %v", err)
		return Event{}, err
	}

	// Write the EventChainItem to the keyValStore
	err = ss.kv.Write(item.Key, data)
	if err != nil {
		log.Fatalf("Error writing item: %v", err)
		return Event{}, err
	}

	return item, err
}

const (
	TrueStr  = "true"
	FalseStr = "false"
)

func (item *Event) CreateDetailsMetaHash() [64]byte {
	// Pre-allocate a buffer to make the hashing process more efficient
	var buffer bytes.Buffer

	// Append Level as an int64 using binary encoding
	levelBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(levelBytes, uint64(item.Level))
	buffer.Write(levelBytes)

	// Append all content hashes
	for _, hash := range item.ContentHashes {
		buffer.Write(hash[:])
	}

	// Append all metadata hashes
	for _, hash := range item.MetadataHashes {
		buffer.Write(hash[:])
	}

	// Append parent event and root event hashes
	buffer.Write(item.HashOfParentEvent[:])
	buffer.Write(item.HashOfRootEvent[:])

	// Append the boolean flags as "true"/"false" strings
	if item.Temporary {
		buffer.WriteString(TrueStr)
	} else {
		buffer.WriteString(FalseStr)
	}

	if item.FullTextSearch {
		buffer.WriteString(TrueStr)
	} else {
		buffer.WriteString(FalseStr)
	}

	return sha512.Sum512(buffer.Bytes())
}

func (ss *Storage) GetEvent(hashOfEvent [64]byte) (Event, error) {
	// Read the EventChainItem from the keyValStore
	value, err := ss.kv.Read(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent))
	if err != nil {
		return Event{}, fmt.Errorf("Error reading Event with Key: %x: %v", string(GenerateKeyFromPrefixAndHash("Event:", hashOfEvent)), err)
	}

	// Deserialize the EventProto using Protocol Buffers
	pbEvent := &EventProto{}
	if err := proto.Unmarshal(value, pbEvent); err != nil {
		return Event{}, fmt.Errorf("Error decoding Event with Key: %x: %v", hashOfEvent, err)
	}
	item := convertFromProtoEvent(pbEvent)

	return item, nil
}

func (ss *Storage) GetAllEvents() ([]Event, error) {
	items, err := ss.kv.GetItemsWithPrefix([]byte("Event:"))
	if err != nil {
		return nil, err
	}

	var events []Event
	for _, item := range items {
		pbEvent := EventProto{}
		if err := proto.Unmarshal(item[1], &pbEvent); err != nil {
			log.Fatalf("Error decoding Event with Key: %x: %v", item[0], err)
			return nil, err
		}

		events = append(events, convertFromProtoEvent(&pbEvent))
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
