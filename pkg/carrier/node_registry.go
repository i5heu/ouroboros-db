package carrier

import (
	"fmt"
	"sync"

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

func (r *nodeRegistry) AddNode( // A
	node interfaces.Node,
	certs []interfaces.NodeCert,
	_ [][]byte,
) error {
	if node.NodeID.IsZero() {
		return fmt.Errorf("node ID must not be zero")
	}
	copyNode := interfaces.Node{
		NodeID:    node.NodeID,
		Addresses: append([]string(nil), node.Addresses...),
		NodeCerts: append([]interfaces.NodeCert(nil), certs...),
	}
	if len(copyNode.NodeCerts) == 0 && len(node.NodeCerts) > 0 {
		copyNode.NodeCerts = append(
			[]interfaces.NodeCert(nil),
			node.NodeCerts...,
		)
	}
	r.mu.Lock()
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
		NodeID:    node.NodeID,
		Addresses: append([]string(nil), node.Addresses...),
		NodeCerts: append([]interfaces.NodeCert(nil), node.NodeCerts...),
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
		})
	}
	return out
}
