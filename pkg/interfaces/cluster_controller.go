package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"google.golang.org/protobuf/proto"
)

// ClusterController is the central control layer for
// node cluster operations. It manages HTTP-style
// message handler registration with TrustScope-based
// access control. Handlers are invoked with peer
// authentication context (NodeID + TrustScope).
type ClusterController interface { // A
	// RegisterHandler associates a MessageType with
	// a handler function and allowed TrustScopes.
	//
	// If the registered message type may be delivered
	// through Carrier unreliable datagrams, the handler
	// must be safe under missing, duplicated, and out-
	// of-order message delivery.
	//
	// Because OuroborosDB is a masterless AP system,
	// handlers must also remain correct during split-
	// brain conditions, partial reachability, and cases
	// where some nodes never receive a message at all.
	// Handler logic should converge via CRDTs or an
	// equivalent partition-tolerant merge model.
	// Use RegisterTypedHandler for strongly typed
	// protobuf handler functions.
	RegisterHandler(
		msgType MessageType,
		scopes []auth.TrustScope,
		handler any,
	) error

	// UnregisterHandler removes the handler for the
	// given MessageType.
	UnregisterHandler(msgType MessageType) error

	// GetHandler returns the registration for the
	// given MessageType, or false if not found.
	GetHandler(
		msgType MessageType,
	) (MessageRegistration, bool)

	// HandleIncomingMessage validates access and
	// dispatches the message to the registered
	// handler. Returns the handler Response or an
	// error if access is denied or no handler exists.
	// The controller does not add replay protection,
	// ordering guarantees, or deduplication for
	// unreliable deliveries; handlers must account for
	// that when such traffic is permitted. It also does
	// not resolve split-brain or partition semantics for
	// the handler; distributed state must converge at the
	// application layer.
	HandleIncomingMessage(
		msg Message,
		peer keys.NodeID,
		peerScope auth.TrustScope,
	) (proto.Message, error)

	// CheckAccess validates whether a peer with the
	// given TrustScope may invoke the handler for
	// the specified MessageType.
	CheckAccess(
		msgType MessageType,
		peerScope auth.TrustScope,
	) AccessDecision

	// GetEffectiveScopes returns the full set of
	// scopes implied by the given scope.
	// ScopeAdmin implies ScopeUser.
	GetEffectiveScopes(
		scope auth.TrustScope,
	) []auth.TrustScope
}
