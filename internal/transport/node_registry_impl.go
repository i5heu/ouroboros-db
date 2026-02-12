package transport

import (
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// nodeRegistryImpl is the thread-safe in-memory
// node registry.
type nodeRegistryImpl struct { // A
	mu    sync.RWMutex
	nodes map[keys.NodeID]*interfaces.NodeInfo
}

// NewNodeRegistry creates a new empty NodeRegistry.
func NewNodeRegistry() NodeRegistry { // A
	return &nodeRegistryImpl{
		nodes: make(
			map[keys.NodeID]*interfaces.NodeInfo,
		),
	}
}

// AddNode registers a peer node with its CA
// signature and trust scope.
func (r *nodeRegistryImpl) AddNode( // A
	peer interfaces.PeerNode,
	caSignature []byte,
	scope auth.TrustScope,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sig := make([]byte, len(caSignature))
	copy(sig, caSignature)

	r.nodes[peer.NodeID] = &interfaces.NodeInfo{
		Peer:        peer,
		CASignature: sig,
		LastSeen:    time.Now(),
		ConnectionStatus: interfaces.
			ConnectionStatusDisconnected,
		TrustScope: scope,
	}
	return nil
}

// RemoveNode deletes a node from the registry.
func (r *nodeRegistryImpl) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.nodes[nodeID]; !ok {
		return fmt.Errorf(
			"node %s not found", nodeID,
		)
	}
	delete(r.nodes, nodeID)
	return nil
}

// GetNode returns the full NodeInfo for a peer.
func (r *nodeRegistryImpl) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.NodeInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.nodes[nodeID]
	if !ok {
		return interfaces.NodeInfo{}, fmt.Errorf(
			"node %s not found", nodeID,
		)
	}
	return *info, nil
}

// GetAllNodes returns all registered peer nodes.
func (r *nodeRegistryImpl) GetAllNodes() []interfaces.PeerNode { // A
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make(
		[]interfaces.PeerNode,
		0,
		len(r.nodes),
	)
	for _, info := range r.nodes {
		out = append(out, info.Peer)
	}
	return out
}

// GetAdminNodes returns nodes with ScopeAdmin trust.
func (r *nodeRegistryImpl) GetAdminNodes() []interfaces.PeerNode { // A
	return r.filterByScope(auth.ScopeAdmin)
}

// GetUserNodes returns nodes with ScopeUser trust.
func (r *nodeRegistryImpl) GetUserNodes() []interfaces.PeerNode { // A
	return r.filterByScope(auth.ScopeUser)
}

// filterByScope returns nodes matching the given
// trust scope.
func (r *nodeRegistryImpl) filterByScope( // A
	scope auth.TrustScope,
) []interfaces.PeerNode {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []interfaces.PeerNode
	for _, info := range r.nodes {
		if info.TrustScope == scope {
			out = append(out, info.Peer)
		}
	}
	return out
}

// UpdateConnectionStatus updates a node's
// connection state.
func (r *nodeRegistryImpl) UpdateConnectionStatus( // A
	nodeID keys.NodeID,
	status interfaces.ConnectionStatus,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.nodes[nodeID]
	if !ok {
		return fmt.Errorf(
			"node %s not found", nodeID,
		)
	}
	info.ConnectionStatus = status
	return nil
}

// UpdateLastSeen refreshes the last-seen timestamp
// for a node.
func (r *nodeRegistryImpl) UpdateLastSeen( // A
	nodeID keys.NodeID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	info, ok := r.nodes[nodeID]
	if !ok {
		return fmt.Errorf(
			"node %s not found", nodeID,
		)
	}
	info.LastSeen = time.Now()
	return nil
}
