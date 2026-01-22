package carrier

import (
	"context"
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// MessageType defines the type of message being sent between nodes.
//
// Message types are organized into categories:
//
// # Data Operations
//   - SealedSlicePayloadRequest: Retrieve sealed slices
//   - ChunkMetaRequest: Query chunk metadata
//   - BlobMetaRequest: Query blob metadata
//
// # Block Distribution
//   - BlockSliceDelivery/Ack: Distribute and acknowledge block slices
//   - BlockAnnouncement: Announce new distributed blocks
//   - MissedBlocksRequest/Response: Catch up on missed blocks
//   - BlockMetadataRequest/Response: Query block metadata
//
// # Cluster Management
//   - Heartbeat: Health monitoring and status
//   - NodeJoinRequest: Request to join cluster
//   - NodeLeaveNotification: Announce departure
//   - NewNodeAnnouncement: Propagate new members
//   - NodeListRequest/Response: Query cluster membership
//
// # Dashboard/Logging
//   - LogSubscribe/Unsubscribe: Remote log streaming
//   - LogEntry: Forwarded log entries
//   - DashboardAnnounce: Dashboard address announcement
//
// # Access Control
//   - UserAuthDecision: Authentication results
type MessageType uint8 // A

const (
	// MessageTypeSealedSlicePayloadRequest requests a sealed slice payload.
	MessageTypeSealedSlicePayloadRequest MessageType = iota + 1
	// MessageTypeChunkMetaRequest requests chunk metadata.
	MessageTypeChunkMetaRequest
	// MessageTypeBlobMetaRequest requests blob metadata.
	MessageTypeBlobMetaRequest
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
	// MessageTypeNodeListRequest requests the list of known nodes.
	MessageTypeNodeListRequest
	// MessageTypeNodeListResponse responds with the list of known nodes.
	MessageTypeNodeListResponse
	// MessageTypeChunkPayloadRequest requests chunk data (reserved).
	MessageTypeChunkPayloadRequest
	// MessageTypeBlobPayloadRequest requests blob data (reserved).
	MessageTypeBlobPayloadRequest

	// Block distribution message types

	// MessageTypeBlockSliceDelivery delivers a block slice to a node.
	MessageTypeBlockSliceDelivery
	// MessageTypeBlockSliceAck acknowledges receipt of a block slice.
	MessageTypeBlockSliceAck
	// MessageTypeBlockAnnouncement announces a new fully-distributed block.
	MessageTypeBlockAnnouncement
	// MessageTypeMissedBlocksRequest requests list of blocks since a timestamp.
	MessageTypeMissedBlocksRequest
	// MessageTypeMissedBlocksResponse responds with list of block hashes.
	MessageTypeMissedBlocksResponse
	// MessageTypeBlockMetadataRequest requests metadata for a block.
	MessageTypeBlockMetadataRequest
	// MessageTypeBlockMetadataResponse responds with block metadata.
	MessageTypeBlockMetadataResponse

	// Dashboard and logging message types

	// MessageTypeLogSubscribe requests to receive logs from a node.
	MessageTypeLogSubscribe
	// MessageTypeLogUnsubscribe stops receiving logs from a node.
	MessageTypeLogUnsubscribe
	// MessageTypeLogEntry contains a forwarded log entry.
	MessageTypeLogEntry
	// MessageTypeDashboardAnnounce announces a node's dashboard address.
	MessageTypeDashboardAnnounce
)

// messageTypeNames maps MessageType values to their string representations.
var messageTypeNames = map[MessageType]string{ // A
	MessageTypeSealedSlicePayloadRequest: "SealedSlicePayloadRequest",
	MessageTypeChunkMetaRequest:          "ChunkMetaRequest",
	MessageTypeBlobMetaRequest:           "BlobMetaRequest",
	MessageTypeHeartbeat:                 "Heartbeat",
	MessageTypeNodeJoinRequest:           "NodeJoinRequest",
	MessageTypeNodeLeaveNotification:     "NodeLeaveNotification",
	MessageTypeUserAuthDecision:          "UserAuthDecision",
	MessageTypeNewNodeAnnouncement:       "NewNodeAnnouncement",
	MessageTypeNodeListRequest:           "NodeListRequest",
	MessageTypeNodeListResponse:          "NodeListResponse",
	MessageTypeChunkPayloadRequest:       "ChunkPayloadRequest",
	MessageTypeBlobPayloadRequest:        "BlobPayloadRequest",
	MessageTypeBlockSliceDelivery:        "BlockSliceDelivery",
	MessageTypeBlockSliceAck:             "BlockSliceAck",
	MessageTypeBlockAnnouncement:         "BlockAnnouncement",
	MessageTypeMissedBlocksRequest:       "MissedBlocksRequest",
	MessageTypeMissedBlocksResponse:      "MissedBlocksResponse",
	MessageTypeBlockMetadataRequest:      "BlockMetadataRequest",
	MessageTypeBlockMetadataResponse:     "BlockMetadataResponse",
	MessageTypeLogSubscribe:              "LogSubscribe",
	MessageTypeLogUnsubscribe:            "LogUnsubscribe",
	MessageTypeLogEntry:                  "LogEntry",
	MessageTypeDashboardAnnounce:         "DashboardAnnounce",
}

// String returns the string representation of a MessageType.
func (mt MessageType) String() string { // A
	if name, ok := messageTypeNames[mt]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(%d)", mt)
}

// Message represents a message exchanged between nodes in the cluster.
//
// Messages are the fundamental communication unit. They consist of a type
// (which determines how the payload is interpreted) and a payload (the
// actual data). The Carrier handles encryption and transport; handlers
// receive decrypted, verified messages.
//
// # Serialization
//
// The Payload format depends on the MessageType. Common formats:
//   - Protocol Buffers for complex structures
//   - Raw bytes for hashes and simple data
//
// # Request/Response Pattern
//
// Many message types follow a request/response pattern:
//   - Sender sends a *Request message
//   - Handler processes and returns a *Response message
//   - Carrier delivers the response to the original sender
type Message struct {
	// Type identifies what kind of message this is.
	// Determines how Payload should be interpreted.
	Type MessageType

	// Payload is the message data, format depends on Type.
	// May be nil for messages that carry no data (e.g., some requests).
	Payload []byte
}

// BroadcastResult contains the result of a broadcast operation.
//
// Since broadcasts go to multiple nodes, some may succeed while others fail.
// BroadcastResult provides detailed information about each node's result.
type BroadcastResult struct {
	// SuccessNodes are nodes that successfully received the message.
	// These nodes acknowledged receipt of the message.
	SuccessNodes []cluster.Node

	// FailedNodes maps failed nodes to their error.
	// Failures may be due to: network issues, node down, timeout, etc.
	FailedNodes map[cluster.NodeID]error
}

// MessageHandler is a callback function for handling incoming messages.
//
// Handlers are registered with the Carrier for specific MessageTypes.
// When a message of that type arrives, the handler is invoked.
//
// # Parameters
//   - ctx: Context for cancellation and deadlines
//   - senderID: The NodeID of the message sender
//   - msg: The received message
//
// # Returns
//   - *Message: Optional response message (nil if no response needed)
//   - error: Error if processing failed
//
// # Thread Safety
//
// Handlers may be called concurrently from multiple goroutines.
// Handlers should not block for extended periods; spawn goroutines
// for long-running operations.
//
// # Example
//
//	handler := func(ctx context.Context, sender cluster.NodeID, msg Message) (*Message, error) {
//	    // Process the message
//	    response := processRequest(msg.Payload)
//	    return &Message{Type: ResponseType, Payload: response}, nil
//	}
//	carrier.RegisterHandler(MessageTypeRequest, handler)
type MessageHandler func(
	ctx context.Context,
	senderID cluster.NodeID,
	msg Message,
) (*Message, error)
