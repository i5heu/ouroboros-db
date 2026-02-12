package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

// ClusterController is the central control layer for
// node cluster operations. It manages HTTP-style
// message handler registration with TrustScope-based
// access control. Handlers are invoked with peer
// authentication context (NodeID + TrustScope).
type ClusterController interface { // A
	// RegisterHandler associates a MessageType with
	// a handler function and allowed TrustScopes.
	RegisterHandler(
		msgType MessageType,
		scopes []auth.TrustScope,
		handler MessageHandler,
	) error

	// UnregisterHandler removes the handler for the
	// given MessageType.
	UnregisterHandler(msgType MessageType) error

	// GetHandler returns the registration for the
	// given MessageType, or false if not found.
	GetHandler(
		msgType MessageType,
	) (HandlerRegistration, bool)

	// HandleIncomingMessage validates access and
	// dispatches the message to the registered
	// handler. Returns the handler Response or an
	// error if access is denied or no handler exists.
	HandleIncomingMessage(
		msg Message,
		peer keys.NodeID,
		peerScope auth.TrustScope,
	) (Response, error)

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
