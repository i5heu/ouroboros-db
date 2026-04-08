package transport

import (
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

type nodeRegistry struct { // A
	mu    sync.RWMutex
	nodes map[keys.NodeID]interfaces.Node
}

func newNodeRegistry() *nodeRegistry { // A
	return &nodeRegistry{
		nodes: make(map[keys.NodeID]interfaces.Node),
	}
}

func (r *nodeRegistry) AddNode( //nolint:cyclop // A: node validation requires multiple checks
	node interfaces.Node,
	certs []interfaces.NodeCert,
	_ [][]byte,
) error {
	if node.NodeID.IsZero() {
		return fmt.Errorf("node ID must not be zero")
	}
	copyNode := interfaces.Node{
		NodeID:           node.NodeID,
		Addresses:        append([]string(nil), node.Addresses...),
		NodeCerts:        append([]interfaces.NodeCert(nil), certs...),
		Role:             node.Role,
		LastSeen:         node.LastSeen,
		ConnectionStatus: node.ConnectionStatus,
	}
	if len(copyNode.NodeCerts) == 0 && len(node.NodeCerts) > 0 {
		copyNode.NodeCerts = append(
			[]interfaces.NodeCert(nil),
			node.NodeCerts...,
		)
	}
	r.mu.Lock()
	if existing, ok := r.nodes[node.NodeID]; ok {
		copyNode.Addresses = compactAddresses(append(
			existing.Addresses,
			copyNode.Addresses...,
		))
		if len(copyNode.NodeCerts) == 0 {
			copyNode.NodeCerts = append(
				[]interfaces.NodeCert(nil),
				existing.NodeCerts...,
			)
		}
		if copyNode.LastSeen.IsZero() {
			copyNode.LastSeen = existing.LastSeen
		}
		if copyNode.Role == interfaces.NodeRoleServer &&
			existing.Role == interfaces.NodeRoleClient {
			copyNode.Role = existing.Role
		}
		if copyNode.ConnectionStatus ==
			interfaces.ConnectionStatusDisconnected &&
			existing.ConnectionStatus !=
				interfaces.ConnectionStatusDisconnected {
			copyNode.ConnectionStatus = existing.ConnectionStatus
		}
	}
	r.nodes[node.NodeID] = copyNode
	r.mu.Unlock()
	return nil
}

func (r *nodeRegistry) RemoveNode( // A
	nodeID keys.NodeID,
) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.nodes[nodeID]; !ok {
		return fmt.Errorf("node not found")
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
		return interfaces.Node{}, fmt.Errorf("node not found")
	}
	return interfaces.Node{
		NodeID:           node.NodeID,
		Addresses:        append([]string(nil), node.Addresses...),
		NodeCerts:        append([]interfaces.NodeCert(nil), node.NodeCerts...),
		Role:             node.Role,
		LastSeen:         node.LastSeen,
		ConnectionStatus: node.ConnectionStatus,
	}, nil
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
		return fmt.Errorf("node not found")
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
		return fmt.Errorf("node not found")
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
		return fmt.Errorf("node not found")
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
		return fmt.Errorf("node not found")
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
			out = append(out, interfaces.Node{
				NodeID:           node.NodeID,
				Addresses:        append([]string(nil), node.Addresses...),
				NodeCerts:        append([]interfaces.NodeCert(nil), node.NodeCerts...),
				Role:             node.Role,
				LastSeen:         node.LastSeen,
				ConnectionStatus: node.ConnectionStatus,
			})
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
			out = append(out, interfaces.Node{
				NodeID:           node.NodeID,
				Addresses:        append([]string(nil), node.Addresses...),
				NodeCerts:        append([]interfaces.NodeCert(nil), node.NodeCerts...),
				Role:             node.Role,
				LastSeen:         node.LastSeen,
				ConnectionStatus: node.ConnectionStatus,
			})
		}
	}
	return out
}

func (r *nodeRegistry) copyNodesLocked() []interfaces.Node { // A
	out := make([]interfaces.Node, 0, len(r.nodes))
	for _, node := range r.nodes {
		out = append(out, interfaces.Node{
			NodeID: node.NodeID,
			Addresses: append(
				[]string(nil), node.Addresses...,
			),
			NodeCerts: append(
				[]interfaces.NodeCert(nil),
				node.NodeCerts...,
			),
			Role:             node.Role,
			LastSeen:         node.LastSeen,
			ConnectionStatus: node.ConnectionStatus,
		})
	}
	return out
}
