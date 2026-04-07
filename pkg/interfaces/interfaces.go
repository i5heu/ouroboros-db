// Package interfaces defines shared transport and
// messaging abstractions used across OuroborosDB
// cluster components.
package interfaces

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
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

type UserMessage = pb.UserMessage // A

type UserMessageResponse = pb.UserMessageResponse // A

type ResponseEmptyPayload = pb.ResponseEmptyPayload // A

// Message is the envelope sent over the Carrier
// transport.
type Message struct { // AC
	Type    MessageType
	Payload []byte
}

// AccessDecision is the result of an authorization
// check against a registered handler.
type AccessDecision struct { // A
	Allowed bool
	Reason  string
}

// MessageHandler is the function signature for
// typed message handlers. Context carries request
// lifecycle and cancellation. Payload is the
// decoded protobuf message for the registered
// MessageType. Peer identifies the authenticated
// remote node. Scope indicates the authenticated
// trust level.
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
	payload proto.Message,
	peer keys.NodeID,
	scope auth.TrustScope,
) (proto.Message, error)

// MessageRegistration stores a registered handler,
// its payload factory, and its access requirements.
type MessageRegistration struct { // A
	MsgType       MessageType
	AllowedScopes []auth.TrustScope
	NewPayload    func() proto.Message
	Handler       MessageHandler
}
