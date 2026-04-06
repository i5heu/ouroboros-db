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
	MessageTypeAuthHandshake
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
//
// Handlers may be invoked for traffic delivered
// over reliable QUIC streams or unreliable QUIC
// datagrams. Handlers for message types that may be
// sent unreliably MUST tolerate missing, duplicated,
// and out-of-order delivery. State-changing logic
// should remain on reliable streams unless the
// message layer adds its own replay, ordering, and
// deduplication guarantees.
//
// This database is a masterless AP system under the
// CAP theorem. Handlers must therefore also tolerate
// split-brain events, temporary partitions, and the
// fact that some or many nodes may never receive a
// given message or may be unreachable at runtime.
// Distributed state changes should be modeled with
// CRDT-style convergence semantics or an equivalent
// merge strategy that remains correct under partial,
// duplicated, delayed, and concurrent delivery.
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
