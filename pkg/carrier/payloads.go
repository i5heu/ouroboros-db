package carrier

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// Payload is implemented by all message payload types.
// It provides type-safe message handling through the Type() method.
type Payload interface { // A
	// Type returns the MessageType for this payload.
	Type() MessageType
}

// --- Log Payloads ---

// LogSubscribePayload is sent when a node wants to subscribe to another
// node's logs.
type LogSubscribePayload struct { // A
	// SubscriberNodeID identifies the node that wants to receive logs.
	SubscriberNodeID cluster.NodeID
	// Levels specifies which log levels to receive. Empty means all levels.
	Levels []string
}

// Type returns the MessageType for LogSubscribePayload.
func (p LogSubscribePayload) Type() MessageType { return MessageTypeLogSubscribe } // A

// LogUnsubscribePayload is sent when a node wants to stop receiving logs.
type LogUnsubscribePayload struct { // A
	// SubscriberNodeID identifies the node that wants to stop receiving logs.
	SubscriberNodeID cluster.NodeID
}

// Type returns the MessageType for LogUnsubscribePayload.
func (p LogUnsubscribePayload) Type() MessageType { return MessageTypeLogUnsubscribe } // A

// LogEntryPayload contains a single log entry forwarded from one node to
// another.
type LogEntryPayload struct { // A
	// SourceNodeID identifies the node that generated the log.
	SourceNodeID cluster.NodeID
	// Timestamp is the Unix timestamp in nanoseconds when the log was created.
	Timestamp int64
	// Level is the log level (debug, info, warn, error).
	Level string
	// Message is the log message text.
	Message string
	// Attributes contains additional key-value pairs from the log record.
	Attributes map[string]string
}

// Type returns the MessageType for LogEntryPayload.
func (p LogEntryPayload) Type() MessageType { return MessageTypeLogEntry } // A

// DashboardAnnouncePayload is sent when a node's dashboard becomes available.
type DashboardAnnouncePayload struct { // A
	// NodeID identifies the node with the dashboard.
	NodeID cluster.NodeID
	// DashboardAddress is the HTTP address (host:port) of the dashboard.
	DashboardAddress string
}

// Type returns the MessageType for DashboardAnnouncePayload.
func (p DashboardAnnouncePayload) Type() MessageType { // A
	return MessageTypeDashboardAnnounce
}

// --- Block Distribution Payloads ---

// BlockSliceDeliveryPayload contains a block slice being delivered to a node.
type BlockSliceDeliveryPayload struct { // A
	// Slice is the BlockSlice being delivered.
	Slice model.BlockSlice
	// RequestID correlates the delivery with its acknowledgment.
	RequestID uint64
}

// Type returns the MessageType for BlockSliceDeliveryPayload.
func (p BlockSliceDeliveryPayload) Type() MessageType { // A
	return MessageTypeBlockSliceDelivery
}

// BlockSliceAckPayload acknowledges receipt of a block slice.
type BlockSliceAckPayload struct { // A
	// SliceHash identifies the slice being acknowledged.
	SliceHash hash.Hash
	// BlockHash identifies the parent block.
	BlockHash hash.Hash
	// RequestID correlates with the original delivery.
	RequestID uint64
	// Success indicates whether the slice was successfully stored.
	Success bool
	// ErrorMsg contains an error message if Success is false.
	ErrorMsg string
}

// Type returns the MessageType for BlockSliceAckPayload.
func (p BlockSliceAckPayload) Type() MessageType { return MessageTypeBlockSliceAck } // A

// BlockAnnouncementPayload announces a newly distributed block to the cluster.
type BlockAnnouncementPayload struct { // A
	// BlockHash uniquely identifies the distributed block.
	BlockHash hash.Hash
	// Timestamp is when the block was created (Unix milliseconds).
	Timestamp int64
	// ChunkCount is the number of chunks in the block.
	ChunkCount uint32
	// VertexCount is the number of vertices in the block.
	VertexCount uint32
}

// Type returns the MessageType for BlockAnnouncementPayload.
func (p BlockAnnouncementPayload) Type() MessageType { // A
	return MessageTypeBlockAnnouncement
}

// MissedBlocksRequestPayload requests blocks distributed since a timestamp.
type MissedBlocksRequestPayload struct { // A
	// SinceTimestamp is the Unix milliseconds timestamp to query from.
	SinceTimestamp int64
}

// Type returns the MessageType for MissedBlocksRequestPayload.
func (p MissedBlocksRequestPayload) Type() MessageType { // A
	return MessageTypeMissedBlocksRequest
}

// MissedBlocksResponsePayload returns a list of distributed block hashes.
type MissedBlocksResponsePayload struct { // A
	// BlockHashes lists the hashes of blocks distributed since the request time.
	BlockHashes []hash.Hash
	// Timestamps contains the creation time for each block (parallel array).
	Timestamps []int64
}

// Type returns the MessageType for MissedBlocksResponsePayload.
func (p MissedBlocksResponsePayload) Type() MessageType { // A
	return MessageTypeMissedBlocksResponse
}

// BlockMetadataRequestPayload requests metadata for a specific block.
type BlockMetadataRequestPayload struct { // A
	// BlockHash identifies the block to get metadata for.
	BlockHash hash.Hash
}

// Type returns the MessageType for BlockMetadataRequestPayload.
func (p BlockMetadataRequestPayload) Type() MessageType { // A
	return MessageTypeBlockMetadataRequest
}

// BlockMetadataResponsePayload returns block metadata.
type BlockMetadataResponsePayload struct { // A
	// Metadata contains the block's structural information.
	Metadata model.BlockMetadata
	// Found indicates whether the block was found.
	Found bool
}

// Type returns the MessageType for BlockMetadataResponsePayload.
func (p BlockMetadataResponsePayload) Type() MessageType { // A
	return MessageTypeBlockMetadataResponse
}

// --- Node Announcement Payloads ---

// NodeAnnouncementPayload contains node information for cluster-wide
// propagation. This is serialized and sent as the payload of
// MessageTypeNewNodeAnnouncement.
type NodeAnnouncementPayload struct { // A
	// Node contains the announced node's information.
	Node cluster.Node
	// Timestamp is when this announcement was created (Unix nanoseconds).
	Timestamp int64
}

// Type returns the MessageType for NodeAnnouncementPayload.
func (p NodeAnnouncementPayload) Type() MessageType { // A
	return MessageTypeNewNodeAnnouncement
}

// NodeListRequestPayload requests the list of known cluster nodes.
type NodeListRequestPayload struct { // A
	// RequestingNodeID identifies the node making the request.
	RequestingNodeID cluster.NodeID
}

// Type returns the MessageType for NodeListRequestPayload.
func (p NodeListRequestPayload) Type() MessageType { // A
	return MessageTypeNodeListRequest
}

// NodeListResponsePayload responds with the list of known cluster nodes.
type NodeListResponsePayload struct { // A
	// Nodes is the list of known cluster nodes.
	Nodes []cluster.Node
}

// Type returns the MessageType for NodeListResponsePayload.
func (p NodeListResponsePayload) Type() MessageType { // A
	return MessageTypeNodeListResponse
}

// --- Cluster Management Payloads ---

// NodeJoinRequestPayload is sent when a node wants to join the cluster.
type NodeJoinRequestPayload struct { // A
	// Node contains the joining node's information.
	Node cluster.Node
}

// Type returns the MessageType for NodeJoinRequestPayload.
func (p NodeJoinRequestPayload) Type() MessageType { // A
	return MessageTypeNodeJoinRequest
}

// NodeLeavePayload is sent when a node leaves the cluster.
type NodeLeavePayload struct { // A
	// NodeID identifies the departing node.
	NodeID cluster.NodeID
}

// Type returns the MessageType for NodeLeavePayload.
func (p NodeLeavePayload) Type() MessageType { // A
	return MessageTypeNodeLeaveNotification
}

// HeartbeatPayload contains health information for cluster monitoring.
type HeartbeatPayload struct { // A
	// NodeID identifies the sending node.
	NodeID cluster.NodeID
	// Timestamp is when this heartbeat was created (Unix nanoseconds).
	Timestamp int64
	// Load represents CPU/system load (0.0-1.0 scale).
	Load float64
	// DiskUsage represents disk usage percentage (0.0-1.0 scale).
	DiskUsage float64
	// MemoryUsage represents memory usage percentage (0.0-1.0 scale).
	MemoryUsage float64
}

// Type returns the MessageType for HeartbeatPayload.
func (p HeartbeatPayload) Type() MessageType { return MessageTypeHeartbeat } // A
