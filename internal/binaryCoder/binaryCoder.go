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
	item := convertFromProtoEvent(pbEvent)

	return item, nil
}

func EventToByte(event types.Event) ([]byte, error) {
	pbEvent := convertToProtoEvent(event)
	data, err := proto.Marshal(pbEvent)
	if err != nil {
		return data, err
	}

	return data, nil
}
