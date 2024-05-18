package binaryCoder

import (
	"github.com/i5heu/ouroboros-db/pkg/types"
)

func EventToProto(event types.Event) (*EventProto, error) {
	return &EventProto{
		EventIdentifier: &EventIdentifierProto{
			EventHash: event.EventIdentifier.EventHash[:],
			EventType: int32(event.EventIdentifier.EventType),
			FastMeta:  event.EventIdentifier.FastMeta.Bytes(),
		},
		Level:          int64(event.Level),
		ContentHashes:  chunkMetasToBytes(event.Content),
		MetadataHashes: chunkMetasToBytes(event.Metadata),
		ParentEvent:    event.ParentEvent[:],
		RootEvent:      event.RootEvent[:],
		Temporary:      bool(event.Temporary),
		FullTextSearch: bool(event.FullTextSearch),
	}, nil
}

func ProtoToEvent(protoEvent *EventProto) (types.Event, error) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventHash: bytesToHash(protoEvent.EventIdentifier.EventHash),
			EventType: types.EventType(protoEvent.EventIdentifier.EventType),
			FastMeta:  bytesToFastMeta(protoEvent.EventIdentifier.FastMeta),
		},
		Level:          types.Level(protoEvent.Level),
		Content:        bytesToChunkMetas(protoEvent.ContentHashes),
		Metadata:       bytesToChunkMetas(protoEvent.MetadataHashes),
		ParentEvent:    bytesToHash(protoEvent.ParentEvent),
		RootEvent:      bytesToHash(protoEvent.RootEvent),
		Temporary:      types.Binary(protoEvent.Temporary),
		FullTextSearch: types.Binary(protoEvent.FullTextSearch),
	}
	return event, nil
}

func chunkMetasToBytes(metas types.ChunkMetaCollection) [][]byte {
	var result [][]byte
	for _, meta := range metas {
		result = append(result, meta.Hash[:])
	}
	return result
}

func bytesToChunkMetas(data [][]byte) types.ChunkMetaCollection {
	var result types.ChunkMetaCollection
	for _, d := range data {
		var hash types.Hash
		copy(hash[:], d)
		result = append(result, types.ChunkMeta{Hash: hash})
	}
	return result
}

func bytesToHash(data []byte) types.Hash {
	var hash types.Hash
	copy(hash[:], data)
	return hash
}

func bytesToFastMeta(data []byte) types.FastMeta {
	var fastMeta types.FastMeta
	fastMeta.FromBytes(data)
	return fastMeta
}
