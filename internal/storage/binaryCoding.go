package storage

// Convert an Event to an EventProto
func convertToProtoEvent(ev Event) *EventProto {
	return &EventProto{
		Key:               ev.Key,
		EventHash:         ev.EventHash[:],
		Level:             ev.Level,
		ContentHashes:     hashesToBytes(ev.ContentHashes),
		MetadataHashes:    hashesToBytes(ev.MetadataHashes),
		HashOfParentEvent: ev.HashOfParentEvent[:],
		HashOfRootEvent:   ev.HashOfRootEvent[:],
		Temporary:         ev.Temporary,
		FullTextSearch:    ev.FullTextSearch,
	}
}

// Convert an EventProto to an Event
func convertFromProtoEvent(pbEvent *EventProto) Event {
	return Event{
		Key:               pbEvent.GetKey(),
		EventHash:         bytesToHash(pbEvent.GetEventHash()),
		Level:             pbEvent.GetLevel(),
		ContentHashes:     bytesToHashes(pbEvent.GetContentHashes()),
		MetadataHashes:    bytesToHashes(pbEvent.GetMetadataHashes()),
		HashOfParentEvent: bytesToHash(pbEvent.GetHashOfParentEvent()),
		HashOfRootEvent:   bytesToHash(pbEvent.GetHashOfRootEvent()),
		Temporary:         pbEvent.GetTemporary(),
		FullTextSearch:    pbEvent.GetFullTextSearch(),
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
