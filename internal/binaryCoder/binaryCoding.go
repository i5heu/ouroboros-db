package binaryCoder

import (
	"github.com/i5heu/ouroboros-db/pkg/types"
)

// Convert an Event to an EventProto
func convertToProtoEvent(ev types.Event) *EventProto {
	return &EventProto{
		Key:               ev.Title,
		EventHash:         ev.EventHash[:],
		Level:             ev.Level,
		ContentHashes:     hashesToBytes(ev.Content),
		MetadataHashes:    hashesToBytes(ev.Metadata),
		HashOfParentEvent: ev.ParentEvent[:],
		HashOfRootEvent:   ev.RootEvent[:],
		Temporary:         ev.Temporary,
		FullTextSearch:    ev.FullTextSearch,
	}
}

// Convert an EventProto to an Event
func convertFromProtoEvent(pbEvent *EventProto) types.Event {
	return types.Event{
		Title:          pbEvent.GetKey(),
		EventHash:      bytesToHash(pbEvent.GetEventHash()),
		Level:          pbEvent.GetLevel(),
		Content:        bytesToHashes(pbEvent.GetContentHashes()),
		Metadata:       bytesToHashes(pbEvent.GetMetadataHashes()),
		ParentEvent:    bytesToHash(pbEvent.GetHashOfParentEvent()),
		RootEvent:      bytesToHash(pbEvent.GetHashOfRootEvent()),
		Temporary:      pbEvent.GetTemporary(),
		FullTextSearch: pbEvent.GetFullTextSearch(),
	}
}

// Convert a slice of [64]byte hashes to a slice of byte slices
func hashesToBytes(hashes [][64]byte) [][]byte {
	result := make([][]byte, len(hashes))
	for i, hash := range hashes {
		result[i] = hash[:]
	}
	return result
}

// Convert a slice of byte slices to a slice of [64]byte hashes
func bytesToHashes(byteSlices [][]byte) [][64]byte {
	hashes := make([][64]byte, len(byteSlices))
	for i, bytes := range byteSlices {
		copy(hashes[i][:], bytes)
	}
	return hashes
}

// Convert a byte slice to a [64]byte hash
func bytesToHash(b []byte) [64]byte {
	var hash [64]byte
	copy(hash[:], b)
	return hash
}
