package carrier

import (
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/carrier"
)

// Registry manages payload type registration and deserialization.
// It provides a centralized way to decode message payloads based on their type.
type Registry struct { // A
	factories map[carrier.MessageType]func([]byte) (carrier.Payload, error)
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry { // A
	return &Registry{
		factories: make(
			map[carrier.MessageType]func([]byte) (carrier.Payload, error),
		),
	}
}

// Register adds a payload type to the registry using generic deserialization.
func Register[T carrier.Payload](
	r *Registry,
	msgType carrier.MessageType,
) { // A
	r.factories[msgType] = func(data []byte) (carrier.Payload, error) {
		return carrier.Deserialize[T](data)
	}
}

// Decode deserializes a payload based on its message type.
func (r *Registry) Decode(
	msgType carrier.MessageType,
	data []byte,
) (carrier.Payload, error) { // A
	factory, ok := r.factories[msgType]
	if !ok {
		return nil, fmt.Errorf("no factory registered for message type %s", msgType)
	}
	return factory(data)
}

// DefaultRegistry returns a registry with all standard payload types
// registered.
func DefaultRegistry() *Registry { // A
	r := NewRegistry()

	// Log payloads
	Register[carrier.LogSubscribePayload](r, carrier.MessageTypeLogSubscribe)
	Register[carrier.LogUnsubscribePayload](r, carrier.MessageTypeLogUnsubscribe)
	Register[carrier.LogEntryPayload](r, carrier.MessageTypeLogEntry)
	Register[carrier.DashboardAnnouncePayload](
		r,
		carrier.MessageTypeDashboardAnnounce,
	)

	// Block distribution payloads
	Register[carrier.BlockSliceDeliveryPayload](
		r,
		carrier.MessageTypeBlockSliceDelivery,
	)
	Register[carrier.BlockSliceAckPayload](r, carrier.MessageTypeBlockSliceAck)
	Register[carrier.BlockAnnouncementPayload](
		r,
		carrier.MessageTypeBlockAnnouncement,
	)
	Register[carrier.MissedBlocksRequestPayload](
		r,
		carrier.MessageTypeMissedBlocksRequest,
	)
	Register[carrier.MissedBlocksResponsePayload](
		r,
		carrier.MessageTypeMissedBlocksResponse,
	)
	Register[carrier.BlockMetadataRequestPayload](
		r,
		carrier.MessageTypeBlockMetadataRequest,
	)
	Register[carrier.BlockMetadataResponsePayload](
		r,
		carrier.MessageTypeBlockMetadataResponse,
	)

	// Node payloads
	Register[carrier.NodeAnnouncementPayload](
		r,
		carrier.MessageTypeNewNodeAnnouncement,
	)
	Register[carrier.NodeListRequestPayload](
		r,
		carrier.MessageTypeNodeListRequest,
	)
	Register[carrier.NodeListResponsePayload](
		r,
		carrier.MessageTypeNodeListResponse,
	)
	Register[carrier.NodeJoinRequestPayload](
		r,
		carrier.MessageTypeNodeJoinRequest,
	)
	Register[carrier.NodeLeavePayload](
		r,
		carrier.MessageTypeNodeLeaveNotification,
	)
	Register[carrier.HeartbeatPayload](r, carrier.MessageTypeHeartbeat)

	return r
}
