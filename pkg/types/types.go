package types

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
)

const (
	TrueStr  = "true"
	FalseStr = "false"
)

type RootEventsIndex struct {
	Title []byte
	Hash  [64]byte
}

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
