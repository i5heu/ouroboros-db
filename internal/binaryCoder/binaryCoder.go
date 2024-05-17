package binaryCoder

import (
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/types"
	"google.golang.org/protobuf/proto"
)

func ByteToEvent(bytes []byte) (types.Event, error) {
	pbEvent := &EventProto{}
	if err := proto.Unmarshal(bytes, pbEvent); err != nil {
		return types.Event{}, fmt.Errorf("Error decoding Event with Key: %v", err)
	}
	item, err := convertFromProtoEvent(pbEvent)
	if err != nil {
		return types.Event{}, fmt.Errorf("Error converting from proto event: %v", err)
	}

	return item, nil
}

func EventToByte(event types.Event) ([]byte, error) {
	pbEvent, err := convertToProtoEvent(event)
	if err != nil {
		return nil, fmt.Errorf("Error converting to proto event: %v", err)
	}
	data, err := proto.Marshal(pbEvent)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling proto event: %v", err)
	}

	return data, nil
}

func convertToProtoEvent(event types.Event) (*EventProto, error) {
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

func convertFromProtoEvent(pbEvent *EventProto) (types.Event, error) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventHash: bytesToHash(pbEvent.EventIdentifier.EventHash),
			EventType: types.EventType(pbEvent.EventIdentifier.EventType),
			FastMeta:  bytesToFastMeta(pbEvent.EventIdentifier.FastMeta),
		},
		Level:          types.Level(pbEvent.Level),
		Content:        bytesToChunkMetas(pbEvent.ContentHashes),
		Metadata:       bytesToChunkMetas(pbEvent.MetadataHashes),
		ParentEvent:    bytesToHash(pbEvent.ParentEvent),
		RootEvent:      bytesToHash(pbEvent.RootEvent),
		Temporary:      types.Binary(pbEvent.Temporary),
		FullTextSearch: types.Binary(pbEvent.FullTextSearch),
	}
	return event, nil
}
