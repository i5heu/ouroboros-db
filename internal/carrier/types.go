// Package carrier implements the Carrier interface for inter-node communication
// in the OuroborosDB cluster.
package carrier

import "fmt"

// MessageType defines the type of message being sent between nodes.
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
	// MessageTypeChunkPayloadRequest requests chunk data (reserved for future
	// use).
	MessageTypeChunkPayloadRequest
	// MessageTypeBlobPayloadRequest requests blob data (reserved for future use).
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

// Slog attribute keys used throughout the carrier package.
const (
	logKeyMessageType  = "messageType"
	logKeyNodeID       = "nodeId"
	logKeyAddress      = "address"
	logKeyError        = "error"
	logKeySuccessCount = "successCount"
	logKeyFailedCount  = "failedCount"
	logKeyViaNode      = "viaNode"
	logKeyAddressCount = "addressCount"
	logKeyNodesRecv    = "nodesReceived"
	logKeyNodesAdded   = "nodesAdded"
	logKeyNodeCount    = "nodeCount"
	logKeyRemoteNodeID = "remoteNodeId"
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
