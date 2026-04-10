package transport

import (
	"errors"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

func copyNode(n *interfaces.Node) interfaces.Node { // A
	return interfaces.Node{
		NodeID:           n.NodeID,
		Addresses:        append([]string(nil), n.Addresses...),
		NodeCerts:        append([]interfaces.NodeCert(nil), n.NodeCerts...),
		Role:             n.Role,
		LastSeen:         n.LastSeen,
		ConnectionStatus: n.ConnectionStatus,
	}
}

type nodeRegistry struct { // A
	mu    sync.RWMutex
	nodes map[keys.NodeID]interfaces.Node
}

func newNodeRegistry() *nodeRegistry { // A
	return &nodeRegistry{
		nodes: make(map[keys.NodeID]interfaces.Node),
	}
}

func (r *nodeRegistry) AddNode( // A: node validation requires multiple checks
	node *interfaces.Node,
	certs []interfaces.NodeCert,
	_ [][]byte,
) error {
	if node.NodeID.IsZero() {
		return errors.New("node ID must not be zero")
	}

	n := interfaces.Node{
		NodeID:           node.NodeID,
		Addresses:        append([]string(nil), node.Addresses...),
		NodeCerts:        append([]interfaces.NodeCert(nil), certs...),
		Role:             node.Role,
		LastSeen:         node.LastSeen,
		ConnectionStatus: node.ConnectionStatus,
	}
	if len(n.NodeCerts) == 0 && len(node.NodeCerts) > 0 {
		n.NodeCerts = append([]interfaces.NodeCert(nil), node.NodeCerts...)
	}

	r.mu.Lock()
	if existing, ok := r.nodes[node.NodeID]; ok {
		n = mergeNode(&existing, &n)
	}
	r.nodes[node.NodeID] = n
	r.mu.Unlock()
	return nil
}

func mergeNode( // A
	existing, incoming *interfaces.Node,
) interfaces.Node {
	if len(incoming.Addresses) == 0 {
		incoming.Addresses = append([]string(nil), existing.Addresses...)
	} else {
		incoming.Addresses = compactAddresses(
			append(existing.Addresses, incoming.Addresses...),
		)
	}
	if len(incoming.NodeCerts) == 0 {
		incoming.NodeCerts = append([]interfaces.NodeCert(nil), existing.NodeCerts...)
	}
	if incoming.LastSeen.IsZero() {
		incoming.LastSeen = existing.LastSeen
	}
	if incoming.Role == interfaces.NodeRoleServer &&
		existing.Role == interfaces.NodeRoleClient {
		incoming.Role = existing.Role
	}
	if incoming.ConnectionStatus == interfaces.ConnectionStatusDisconnected &&
		existing.ConnectionStatus != interfaces.ConnectionStatusDisconnected {
		incoming.ConnectionStatus = existing.ConnectionStatus
	}
	return *incoming
}

func (r *nodeRegistry) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; !ok {
		return errors.New("node not found")
	}
	delete(r.nodes, nodeID)
	return nil
}

func (r *nodeRegistry) GetNode( // A
	nodeID keys.NodeID,
) (interfaces.Node, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	node, ok := r.nodes[nodeID]
	if !ok {
		return interfaces.Node{}, errors.New("node not found")
	}
	return copyNode(&node), nil
}

func (r *nodeRegistry) GetAllNodes() []interfaces.Node { // A
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.copyNodesLocked()
}

func (r *nodeRegistry) GetAdminNodes() []interfaces.Node { // A
	return r.GetAllNodes()
}

func (r *nodeRegistry) GetUserNodes() []interfaces.Node { // A
	return r.GetAllNodes()
}

func (r *nodeRegistry) UpdateLastSeen( // A
	nodeID keys.NodeID,
	seenAt time.Time,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[nodeID]
	if !ok {
		return errors.New("node not found")
	}
	node.LastSeen = seenAt
	r.nodes[nodeID] = node
	return nil
}

func (r *nodeRegistry) SetStatus( // A
	nodeID keys.NodeID,
	status interfaces.ConnectionStatus,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[nodeID]
	if !ok {
		return errors.New("node not found")
	}
	node.ConnectionStatus = status
	r.nodes[nodeID] = node
	return nil
}

func (r *nodeRegistry) SetRole( // A
	nodeID keys.NodeID,
	role interfaces.NodeRole,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[nodeID]
	if !ok {
		return errors.New("node not found")
	}
	node.Role = role
	r.nodes[nodeID] = node
	return nil
}

func (r *nodeRegistry) MergeAddresses( // A
	nodeID keys.NodeID,
	addresses []string,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	node, ok := r.nodes[nodeID]
	if !ok {
		return errors.New("node not found")
	}
	node.Addresses = compactAddresses(append(
		node.Addresses,
		addresses...,
	))
	r.nodes[nodeID] = node
	return nil
}

func (r *nodeRegistry) GetUnreachableNodes() []interfaces.Node { // A
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]interfaces.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.ConnectionStatus !=
			interfaces.ConnectionStatusConnected {
			out = append(out, copyNode(&node))
		}
	}
	return out
}

func (r *nodeRegistry) GetServerNodes() []interfaces.Node { // A
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]interfaces.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		if node.Role != interfaces.NodeRoleClient {
			out = append(out, copyNode(&node))
		}
	}
	return out
}

func (r *nodeRegistry) copyNodesLocked() []interfaces.Node { // A
	out := make([]interfaces.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		out = append(out, copyNode(&node))
	}
	return out
}
