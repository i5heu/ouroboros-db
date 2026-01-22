// Package cluster provides the implementation of cluster management for
// OuroborosDB.
package cluster

import (
	"fmt"
	"sync"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// DefaultCluster is the default implementation of the Cluster interface.
type DefaultCluster struct {
	mu    sync.RWMutex
	nodes map[cluster.NodeID]cluster.Node
}

// New creates a new DefaultCluster instance.
func New() *DefaultCluster {
	return &DefaultCluster{
		nodes: make(map[cluster.NodeID]cluster.Node),
	}
}

// GetNodes returns all nodes currently in the cluster.
func (c *DefaultCluster) GetNodes() []cluster.Node {
	c.mu.RLock()
	defer c.mu.RUnlock()

	nodes := make([]cluster.Node, 0, len(c.nodes))
	for _, node := range c.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// AddNode adds a node to the cluster.
func (c *DefaultCluster) AddNode(node cluster.Node) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if node.NodeID == "" {
		return fmt.Errorf("cluster: node ID cannot be empty")
	}

	c.nodes[node.NodeID] = node
	return nil
}

// RemoveNode removes a node from the cluster.
func (c *DefaultCluster) RemoveNode(nodeID cluster.NodeID) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.nodes[nodeID]; !exists {
		return fmt.Errorf("cluster: node %s not found", nodeID)
	}

	delete(c.nodes, nodeID)
	return nil
}

// GetNode returns a specific node by its ID.
func (c *DefaultCluster) GetNode(nodeID cluster.NodeID) (cluster.Node, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	node, exists := c.nodes[nodeID]
	if !exists {
		return cluster.Node{}, fmt.Errorf("cluster: node %s not found", nodeID)
	}

	return node, nil
}

// Ensure DefaultCluster implements the Cluster interface.
var _ cluster.Cluster = (*DefaultCluster)(nil)
