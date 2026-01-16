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
//   - BlockSliceRequest/Response: Retrieve block slices
//   - ChunkMetaRequest: Query chunk metadata
//   - VertexMetaRequest: Query vertex metadata
//   - KeyEntryRequest/Response: Access control queries
//   - BlockSyncRequest: Synchronization operations
//
// # Cluster Management
//   - Heartbeat: Health monitoring and status
//   - NodeJoinRequest: Request to join cluster
//   - NodeLeaveNotification: Announce departure
//   - NewNodeAnnouncement: Propagate new members
//
// # Access Control
//   - UserAuthDecision: Authentication results
type MessageType uint8

const (
	// MessageTypeBlockSliceRequest requests a block slice payload.
	// Payload: BlockHash + RSSliceIndex
	// Response: MessageTypeBlockSliceResponse with the slice data
	MessageTypeBlockSliceRequest MessageType = iota + 1

	// MessageTypeBlockSliceResponse responds with a block slice payload.
	// Payload: Serialized BlockSlice
	MessageTypeBlockSliceResponse

	// MessageTypeChunkMetaRequest requests chunk metadata (without content).
	// Payload: ChunkHash
	// Response: Chunk metadata (size, block location, etc.)
	MessageTypeChunkMetaRequest

	// MessageTypeVertexMetaRequest requests vertex metadata.
	// Payload: VertexHash
	// Response: Vertex metadata (Parent, Created, ChunkHashes)
	MessageTypeVertexMetaRequest

	// MessageTypeHeartbeat is used for health monitoring and cluster status.
	// Payload: Node status (load, disk usage, memory, etc.)
	// Sent periodically to maintain cluster health information.
	MessageTypeHeartbeat

	// MessageTypeNodeJoinRequest is sent when a node wants to join the cluster.
	// Payload: NodeCert for authentication
	// Response: Current cluster membership or rejection
	MessageTypeNodeJoinRequest

	// MessageTypeNodeLeaveNotification is sent when a node leaves the cluster.
	// Payload: NodeID of the departing node
	// No response expected; informational only.
	MessageTypeNodeLeaveNotification

	// MessageTypeUserAuthDecision communicates authentication decisions.
	// Payload: User identity, decision (allow/deny), permissions
	// Used for distributed access control decisions.
	MessageTypeUserAuthDecision

	// MessageTypeNewNodeAnnouncement announces a new node to the cluster.
	// Payload: Full Node information (ID, Addresses, Cert)
	// Broadcast to all nodes when a new member joins.
	MessageTypeNewNodeAnnouncement

	// MessageTypeKeyEntryRequest requests key entries for a chunk.
	// Payload: ChunkHash + PubKeyHash
	// Response: MessageTypeKeyEntryResponse with KeyEntry(s)
	MessageTypeKeyEntryRequest

	// MessageTypeKeyEntryResponse responds with key entries.
	// Payload: Serialized KeyEntry(s)
	MessageTypeKeyEntryResponse

	// MessageTypeBlockSyncRequest requests block synchronization.
	// Payload: BlockHash or hash range for sync
	// Used during rebalancing and recovery operations.
	MessageTypeBlockSyncRequest
)

// messageTypeNames maps MessageType values to their string representations.
var messageTypeNames = map[MessageType]string{
	MessageTypeBlockSliceRequest:     "BlockSliceRequest",
	MessageTypeBlockSliceResponse:    "BlockSliceResponse",
	MessageTypeChunkMetaRequest:      "ChunkMetaRequest",
	MessageTypeVertexMetaRequest:     "VertexMetaRequest",
	MessageTypeHeartbeat:             "Heartbeat",
	MessageTypeNodeJoinRequest:       "NodeJoinRequest",
	MessageTypeNodeLeaveNotification: "NodeLeaveNotification",
	MessageTypeUserAuthDecision:      "UserAuthDecision",
	MessageTypeNewNodeAnnouncement:   "NewNodeAnnouncement",
	MessageTypeKeyEntryRequest:       "KeyEntryRequest",
	MessageTypeKeyEntryResponse:      "KeyEntryResponse",
	MessageTypeBlockSyncRequest:      "BlockSyncRequest",
}

// String returns the string representation of a MessageType.
func (mt MessageType) String() string {
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
