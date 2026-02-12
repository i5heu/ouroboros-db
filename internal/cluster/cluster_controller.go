// Package cluster implements the ClusterController,
// the per-node control layer for handler registration,
// TrustScope-based access control, and message
// dispatch.
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// clusterController is the concrete implementation
// of interfaces.ClusterController.
type clusterController struct { // A
	mu       sync.RWMutex
	handlers map[interfaces.MessageType]*interfaces.HandlerRegistration
	carrier  interfaces.Carrier
	// LOGGER: Using slog directly because
	// ClusterController is a dependency of
	// ClusterLog (via Carrier). Using
	// pkg/clusterlog here would risk a
	// subscription loop.
	logger *slog.Logger
}

// NewClusterController creates a new
// ClusterController backed by the given Carrier.
func NewClusterController( // A
	carrier interfaces.Carrier,
	logger *slog.Logger,
) (*clusterController, error) {
	if carrier == nil {
		return nil, fmt.Errorf(
			"carrier must not be nil",
		)
	}
	if logger == nil {
		return nil, fmt.Errorf(
			"logger must not be nil",
		)
	}
	return &clusterController{
		handlers: make(
			map[interfaces.MessageType]*interfaces.HandlerRegistration,
		),
		carrier: carrier,
		logger:  logger,
	}, nil
}

// RegisterHandler associates a MessageType with a
// handler function and allowed TrustScopes. Returns
// an error if the MessageType is already registered
// or if the inputs are invalid.
func (cc *clusterController) RegisterHandler( // A
	msgType interfaces.MessageType,
	scopes []auth.TrustScope,
	handler interfaces.MessageHandler,
) error {
	if handler == nil {
		return fmt.Errorf(
			"handler must not be nil",
		)
	}
	if len(scopes) == 0 {
		return fmt.Errorf(
			"at least one scope is required",
		)
	}

	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, exists := cc.handlers[msgType]; exists {
		return fmt.Errorf(
			"handler already registered for "+
				"message type %d",
			msgType,
		)
	}

	cc.handlers[msgType] = &interfaces.HandlerRegistration{
		MsgType:       msgType,
		AllowedScopes: scopes,
		Handler:       handler,
	}
	return nil
}

// UnregisterHandler removes the handler for the
// given MessageType. Returns an error if no handler
// is registered for that type.
func (cc *clusterController) UnregisterHandler( // A
	msgType interfaces.MessageType,
) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if _, exists := cc.handlers[msgType]; !exists {
		return fmt.Errorf(
			"no handler registered for "+
				"message type %d",
			msgType,
		)
	}
	delete(cc.handlers, msgType)
	return nil
}

// GetHandler returns the HandlerRegistration for the
// given MessageType, or false if not found.
func (cc *clusterController) GetHandler( // A
	msgType interfaces.MessageType,
) (interfaces.HandlerRegistration, bool) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	reg, ok := cc.handlers[msgType]
	if !ok {
		return interfaces.HandlerRegistration{}, false
	}
	return *reg, true
}

// GetEffectiveScopes returns the full set of scopes
// implied by the given scope. ScopeAdmin implies
// ScopeUser (admin has all user permissions).
func (cc *clusterController) GetEffectiveScopes( // A
	scope auth.TrustScope,
) []auth.TrustScope {
	switch scope {
	case auth.ScopeAdmin:
		return []auth.TrustScope{
			auth.ScopeAdmin,
			auth.ScopeUser,
		}
	case auth.ScopeUser:
		return []auth.TrustScope{
			auth.ScopeUser,
		}
	default:
		return nil
	}
}

// CheckAccess validates whether a peer with the
// given TrustScope may invoke the handler registered
// for msgType. The peer's effective scopes are
// compared against the handler's AllowedScopes using
// OR logic (peer needs ANY matching scope).
func (cc *clusterController) CheckAccess( // A
	msgType interfaces.MessageType,
	peerScope auth.TrustScope,
) interfaces.AccessDecision {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	reg, ok := cc.handlers[msgType]
	if !ok {
		return interfaces.AccessDecision{
			Allowed: false,
			Reason: fmt.Sprintf(
				"no handler registered for "+
					"message type %d",
				msgType,
			),
		}
	}

	effective := cc.getEffectiveScopesUnlocked(
		peerScope,
	)
	for _, es := range effective {
		for _, as := range reg.AllowedScopes {
			if es == as {
				return interfaces.AccessDecision{
					Allowed: true,
				}
			}
		}
	}

	return interfaces.AccessDecision{
		Allowed: false,
		Reason: fmt.Sprintf(
			"peer scope %s does not match any "+
				"allowed scope for message type %d",
			peerScope,
			msgType,
		),
	}
}

// HandleIncomingMessage validates access and
// dispatches the message to the registered handler.
// Returns the handler Response or an error if access
// is denied or no handler exists.
func (cc *clusterController) HandleIncomingMessage( // A
	msg interfaces.Message,
	peer keys.NodeID,
	peerScope auth.TrustScope,
) (interfaces.Response, error) {
	decision := cc.CheckAccess(
		msg.Type,
		peerScope,
	)
	if !decision.Allowed {
		return interfaces.Response{}, fmt.Errorf(
			"access denied: %s", decision.Reason,
		)
	}

	cc.mu.RLock()
	reg := cc.handlers[msg.Type]
	cc.mu.RUnlock()

	ctx := context.Background()
	return reg.Handler(ctx, msg, peer, peerScope)
}

// getEffectiveScopesUnlocked is the internal helper
// used by CheckAccess (which already holds the lock).
func (cc *clusterController) getEffectiveScopesUnlocked( // A
	scope auth.TrustScope,
) []auth.TrustScope {
	switch scope {
	case auth.ScopeAdmin:
		return []auth.TrustScope{
			auth.ScopeAdmin,
			auth.ScopeUser,
		}
	case auth.ScopeUser:
		return []auth.TrustScope{
			auth.ScopeUser,
		}
	default:
		return nil
	}
}
