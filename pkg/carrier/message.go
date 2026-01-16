package carrier

import (
	"context"
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// MessageType defines the type of message being sent between nodes.
type MessageType uint8

const (
	// MessageTypeBlockSliceRequest requests a block slice payload.
	MessageTypeBlockSliceRequest MessageType = iota + 1

	// MessageTypeBlockSliceResponse responds with a block slice payload.
	MessageTypeBlockSliceResponse

	// MessageTypeChunkMetaRequest requests chunk metadata.
	MessageTypeChunkMetaRequest

	// MessageTypeVertexMetaRequest requests vertex metadata.
	MessageTypeVertexMetaRequest

	// MessageTypeHeartbeat is used for health monitoring and cluster status.
	MessageTypeHeartbeat

	// MessageTypeNodeJoinRequest is sent when a node wants to join the cluster.
	MessageTypeNodeJoinRequest

	// MessageTypeNodeLeaveNotification is sent when a node leaves the cluster.
	MessageTypeNodeLeaveNotification

	// MessageTypeUserAuthDecision communicates authentication decisions.
	MessageTypeUserAuthDecision

	// MessageTypeNewNodeAnnouncement announces a new node to the cluster.
	MessageTypeNewNodeAnnouncement

	// MessageTypeKeyEntryRequest requests key entries for a chunk.
	MessageTypeKeyEntryRequest

	// MessageTypeKeyEntryResponse responds with key entries.
	MessageTypeKeyEntryResponse

	// MessageTypeBlockSyncRequest requests block synchronization.
	MessageTypeBlockSyncRequest
)

// messageTypeNames maps MessageType values to their string representations.
var messageTypeNames = map[MessageType]string{
	MessageTypeBlockSliceRequest:      "BlockSliceRequest",
	MessageTypeBlockSliceResponse:     "BlockSliceResponse",
	MessageTypeChunkMetaRequest:       "ChunkMetaRequest",
	MessageTypeVertexMetaRequest:      "VertexMetaRequest",
	MessageTypeHeartbeat:              "Heartbeat",
	MessageTypeNodeJoinRequest:        "NodeJoinRequest",
	MessageTypeNodeLeaveNotification:  "NodeLeaveNotification",
	MessageTypeUserAuthDecision:       "UserAuthDecision",
	MessageTypeNewNodeAnnouncement:    "NewNodeAnnouncement",
	MessageTypeKeyEntryRequest:        "KeyEntryRequest",
	MessageTypeKeyEntryResponse:       "KeyEntryResponse",
	MessageTypeBlockSyncRequest:       "BlockSyncRequest",
}

// String returns the string representation of a MessageType.
func (mt MessageType) String() string {
	if name, ok := messageTypeNames[mt]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", mt)
}

// Message represents a message exchanged between nodes in the cluster.
type Message struct {
	// Type identifies what kind of message this is.
	Type MessageType

	// Payload is the message data, format depends on Type.
	Payload []byte
}

// BroadcastResult contains the result of a broadcast operation.
type BroadcastResult struct {
	// SuccessNodes are nodes that successfully received the message.
	SuccessNodes []cluster.Node

	// FailedNodes maps failed nodes to their error.
	FailedNodes map[cluster.NodeID]error
}

// MessageHandler is a callback function for handling incoming messages.
// It receives the sender's node ID, the message, and should return a response
// message (or nil if no response is needed) and any error.
type MessageHandler func(
	ctx context.Context,
	senderID cluster.NodeID,
	msg Message,
) (*Message, error)
