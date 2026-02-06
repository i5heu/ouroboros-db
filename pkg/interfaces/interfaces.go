// Package interfaces defines shared transport and
// messaging abstractions used across OuroborosDB
// cluster components.
package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// MessageType enumerates the kinds of messages
// exchanged between cluster nodes.
type MessageType int // AC

const ( // AC
	MessageTypeBlockSliceRequest MessageType = iota
	MessageTypeBlockSliceResponse
	MessageTypeChunkMetaRequest
	MessageTypeVertexMetaRequest
	MessageTypeHeartbeat
	MessageTypeNodeJoinRequest
	MessageTypeNodeLeaveNotification
	MessageTypeUserAuthDecision
	MessageTypeNewNodeAnnouncement
	MessageTypeKeyEntryRequest
	MessageTypeKeyEntryResponse
	MessageTypeBlockSyncRequest
	MessageTypeLogPush
	MessageTypeLogSendResponse
)

// Message is the envelope sent over the Carrier
// transport.
type Message struct { // AC
	Type    MessageType
	Payload []byte
}

// NodeCert holds the cryptographic certificate
// material for a cluster node.
type NodeCert struct { // AC
	PublicKey []byte
}

// Node represents a cluster member.
type Node struct { // AC
	NodeID    keys.NodeID
	Addresses []string
	Cert      NodeCert
}

// Carrier abstracts the network transport used to
// communicate between cluster nodes.
type Carrier interface { // AC
	GetNodes() []Node
	Broadcast(
		message Message,
	) (success []Node, err error)
	SendMessageToNode(
		nodeID keys.NodeID,
		message Message,
	) error
	JoinCluster(
		clusterNode Node,
		cert NodeCert,
	) error
	LeaveCluster(clusterNode Node) error
}
