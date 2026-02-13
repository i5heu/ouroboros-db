// Package interfaces defines shared transport and
// messaging abstractions used across OuroborosDB
// cluster components.
package interfaces

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
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
	MessageTypeUserMessage
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

// UserMessagePayload is the payload for
// MessageTypeUserMessage.
type UserMessagePayload struct { // A
	From string `json:"from"`
	Text string `json:"text"`
}

// Message is the envelope sent over the Carrier
// transport.
type Message struct { // AC
	Type    MessageType
	Payload []byte
}

// Response is the handler response envelope returned
// to the caller after message processing.
type Response struct { // A
	Payload  []byte
	Error    error
	Metadata map[string]string
}

// AccessDecision is the result of an authorization
// check against a registered handler.
type AccessDecision struct { // A
	Allowed bool
	Reason  string
}

// MessageHandler is the function signature for
// HTTP-style message handlers. Context carries
// request lifecycle and cancellation. Message
// contains the deserialized request payload. Peer
// identifies the authenticated remote node. Scope
// indicates the authenticated trust level.
type MessageHandler func( // A
	ctx context.Context,
	msg Message,
	peer keys.NodeID,
	scope auth.TrustScope,
) (Response, error)

// HandlerRegistration stores a registered handler
// with its access requirements.
type HandlerRegistration struct { // A
	MsgType       MessageType
	AllowedScopes []auth.TrustScope
	Handler       MessageHandler
}

// Carrier abstracts the network transport used to
// communicate between cluster nodes.
type Carrier interface { // A
	GetNodes() []PeerNode
	GetNode(
		nodeID keys.NodeID,
	) (PeerNode, error)
	BroadcastReliable(
		message Message,
	) (
		success []PeerNode,
		failed []PeerNode,
		err error,
	)
	SendMessageToNodeReliable(
		nodeID keys.NodeID,
		message Message,
	) error
	BroadcastUnreliable(
		message Message,
	) (attempted []PeerNode)
	SendMessageToNodeUnreliable(
		nodeID keys.NodeID,
		message Message,
	) error
	Broadcast(
		message Message,
	) (success []PeerNode, err error)
	SendMessageToNode(
		nodeID keys.NodeID,
		message Message,
	) error
	JoinCluster(
		clusterNode PeerNode,
		cert *auth.NodeCert,
	) error
	LeaveCluster(clusterNode PeerNode) error
	RemoveNode(nodeID keys.NodeID) error
	IsConnected(nodeID keys.NodeID) bool
}
