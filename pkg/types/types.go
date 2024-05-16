package types

import (
	"bytes"
	"crypto/sha512"
	"fmt"
)

type EventIdentifier struct {
	EventHash Hash      // SHA-512 hash of all other fields in this struct
	EventType EventType // Defines what behavior the event event should have.
	FastMeta  FastMeta  // FastMeta allows for metadata to be stored in the description of the event itself, what leads to faster access without indexing. The maximum size of the FastMeta is 128kb
}

// Event represents an event in the EventChain, the absolute top of a EventChain is a RootEvent, look at rootEvents.go
type Event struct {
	EventIdentifier                     // This will be used to identify the event it includes  EventHash, EventType, FastMeta
	Level           Level               // unix timestamp of creation time UTC
	Metadata        ChunkMetaCollection // If no metadata is present the metadata of the the next previous event with metadata is used
	Content         ChunkMetaCollection // all hashes in a certain order that allow to reconstruct the content from the values in badgerDB
	ParentEvent     Hash                // the event this item is the child of
	RootEvent       Hash                // the first event that marks a now EventChain
	Temporary       Binary              // no non-Temporary event can have a Temporary event as Parent, Temporary events will be removed after some conditions are met, if one deletes a Temporary event all its children will be deleted too
	FullTextSearch  Binary              // if true the ContentHashes and MetadataHashes of the event will be used for full-text search, Important: this only applies to the event itself, not to its children
}

func (item *Event) CreateDetailsMetaHash() Hash {
	// Pre-allocate a buffer to make the hashing process more efficient
	var buffer bytes.Buffer

	// Append Event type as an int using decimal encoding
	buffer.WriteString(item.EventType.String())

	// Append Level as an int64 using binary encoding
	buffer.Write(item.Level.Bytes())

	// Append Title as a string
	buffer.Write(item.FastMeta.Hash().Bytes())

	buffer.Write(item.Metadata.Hash().Bytes())
	buffer.Write(item.Content.Hash().Bytes())

	// Append parent event and root event hashes
	buffer.Write(item.ParentEvent.Bytes())
	buffer.Write(item.RootEvent.Bytes())

	buffer.Write(item.Temporary.Bytes())
	buffer.Write(item.FullTextSearch.Bytes())

	return sha512.Sum512(buffer.Bytes())
}

const eventPrefix = "Event"

func (item *Event) Key() EventKeyBytes {
	buf := bytes.NewBufferString(eventPrefix)
	buf.WriteString(":")
	buf.Write(item.EventType.Bytes()) // 1 byte
	buf.Write(item.EventHash.Bytes()) // 64 bytes
	buf.Write(item.FastMeta.Bytes())  // variable length
	return buf.Bytes()
}

type EventKeyBytes []byte

func (ekb EventKeyBytes) Deserialize() (*EventIdentifier, error) {
	// The known lengths
	const eventTypeLength = 1
	const eventHashLength = 64

	// Verify the prefix and minimum length
	prefixLength := len(eventPrefix) + 1 // Including the ':'
	if len(ekb) < prefixLength+eventTypeLength+eventHashLength {
		return nil, fmt.Errorf("invalid EventKeyBytes format")
	}

	// Check the prefix
	if string(ekb[:prefixLength-1]) != eventPrefix {
		return nil, fmt.Errorf("invalid EventKeyBytes format")
	}

	evId := &EventIdentifier{}

	// Extract and set the EventType
	eventTypeBytes := ekb[prefixLength : prefixLength+eventTypeLength]
	err := evId.EventType.FromBytes(eventTypeBytes)
	if err != nil {
		return nil, err
	}

	// Extract and set the EventHash
	eventHashBytes := ekb[prefixLength+eventTypeLength : prefixLength+eventTypeLength+eventHashLength]
	err = evId.EventHash.HashFromBytes(eventHashBytes)
	if err != nil {
		return nil, err
	}

	// Extract and set the FastMeta
	fastMetaBytes := ekb[prefixLength+eventTypeLength+eventHashLength:]
	err = evId.FastMeta.FromBytes(FastMetaBytes(fastMetaBytes))
	if err != nil {
		return nil, err
	}

	return evId, nil
}

type ChunkMetaCollection []ChunkMeta

func (c ChunkMetaCollection) Hash() Hash {
	var buffer bytes.Buffer

	for _, chunk := range c {
		buffer.Write(chunk.Hash.Bytes())
	}

	hash := sha512.Sum512(buffer.Bytes())

	return hash
}
