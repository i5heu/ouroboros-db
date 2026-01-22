// Package health provides health monitoring implementations for OuroborosDB.
package health

import (
	"context"
	"time"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
	"github.com/i5heu/ouroboros-db/pkg/monitor"
)

// DefaultClusterMonitor implements the ClusterMonitor interface.
type DefaultClusterMonitor struct {
	statuses  map[cluster.NodeID]monitor.NodeStatus
	callbacks []monitor.HealthCallback
	tracker   *DefaultNodeAvailabilityTracker
}

// NewClusterMonitor creates a new DefaultClusterMonitor instance.
func NewClusterMonitor() *DefaultClusterMonitor {
	return &DefaultClusterMonitor{
		statuses:  make(map[cluster.NodeID]monitor.NodeStatus),
		callbacks: make([]monitor.HealthCallback, 0),
		tracker:   NewNodeAvailabilityTracker(),
	}
}

// MonitorNodeHealth checks the health of all nodes in the cluster.
func (m *DefaultClusterMonitor) MonitorNodeHealth(ctx context.Context) error {
	// Implementation will perform health checks on all known nodes
	// This is a placeholder for the actual implementation
	return nil
}

// GetNodeStatus returns the current status of a specific node.
func (m *DefaultClusterMonitor) GetNodeStatus(
	ctx context.Context,
	nodeID cluster.NodeID,
) (monitor.NodeStatus, error) {
	status, exists := m.statuses[nodeID]
	if !exists {
		return monitor.NodeStatus{
			NodeID:    nodeID,
			Available: false,
		}, nil
	}
	return status, nil
}

// GetClusterHealth returns the overall health of the cluster.
func (m *DefaultClusterMonitor) GetClusterHealth(
	ctx context.Context,
) (monitor.ClusterHealth, error) {
	available := 0
	unavailable := 0
	for _, status := range m.statuses {
		if status.Available {
			available++
		} else {
			unavailable++
		}
	}

	return monitor.ClusterHealth{
		Healthy:           available > 0,
		TotalNodes:        len(m.statuses),
		AvailableNodes:    available,
		UnavailableNodes:  unavailable,
		ReplicationFactor: 1, // Default, should be configurable
	}, nil
}

// RegisterHealthCallback registers a callback for health status changes.
func (m *DefaultClusterMonitor) RegisterHealthCallback(
	callback monitor.HealthCallback,
) {
	m.callbacks = append(m.callbacks, callback)
}

// UpdateNodeStatus updates the status of a node and notifies callbacks.
func (m *DefaultClusterMonitor) UpdateNodeStatus(
	nodeID cluster.NodeID,
	newStatus monitor.NodeStatus,
) {
	oldStatus := m.statuses[nodeID]
	m.statuses[nodeID] = newStatus
	callbacks := make([]monitor.HealthCallback, len(m.callbacks))
	copy(callbacks, m.callbacks)

	// Notify callbacks
	for _, cb := range callbacks {
		cb(nodeID, oldStatus, newStatus)
	}
}

// Ensure DefaultClusterMonitor implements the ClusterMonitor interface.
var _ monitor.ClusterMonitor = (*DefaultClusterMonitor)(nil)

// DefaultNodeAvailabilityTracker implements the NodeAvailabilityTracker
// interface.
type DefaultNodeAvailabilityTracker struct {
	available   map[cluster.NodeID]bool
	nodes       map[cluster.NodeID]cluster.Node
	lastChecked map[cluster.NodeID]time.Time
}

// NewNodeAvailabilityTracker creates a new DefaultNodeAvailabilityTracker
// instance.
func NewNodeAvailabilityTracker() *DefaultNodeAvailabilityTracker {
	return &DefaultNodeAvailabilityTracker{
		available:   make(map[cluster.NodeID]bool),
		nodes:       make(map[cluster.NodeID]cluster.Node),
		lastChecked: make(map[cluster.NodeID]time.Time),
	}
}

// TrackAvailability starts tracking availability for all known nodes.
func (t *DefaultNodeAvailabilityTracker) TrackAvailability(
	ctx context.Context,
) error {
	// Implementation will start a background goroutine to track availability
	// This is a placeholder for the actual implementation
	return nil
}

// IsNodeAvailable returns whether a node is currently available.
func (t *DefaultNodeAvailabilityTracker) IsNodeAvailable(
	nodeID cluster.NodeID,
) bool {
	return t.available[nodeID]
}

// GetAvailableNodes returns all currently available nodes.
func (t *DefaultNodeAvailabilityTracker) GetAvailableNodes() []cluster.Node {
	var nodes []cluster.Node
	for nodeID, isAvailable := range t.available {
		if isAvailable {
			if node, exists := t.nodes[nodeID]; exists {
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

// GetUnavailableNodes returns all currently unavailable nodes.
func (t *DefaultNodeAvailabilityTracker) GetUnavailableNodes() []cluster.Node {
	var nodes []cluster.Node
	for nodeID, isAvailable := range t.available {
		if !isAvailable {
			if node, exists := t.nodes[nodeID]; exists {
				nodes = append(nodes, node)
			}
		}
	}
	return nodes
}

// MarkNodeUnavailable manually marks a node as unavailable.
func (t *DefaultNodeAvailabilityTracker) MarkNodeUnavailable(
	nodeID cluster.NodeID,
) {
	t.available[nodeID] = false
}

// MarkNodeAvailable manually marks a node as available.
func (t *DefaultNodeAvailabilityTracker) MarkNodeAvailable(
	nodeID cluster.NodeID,
) {
	t.available[nodeID] = true
}

// AddNode adds a node to track.
func (t *DefaultNodeAvailabilityTracker) AddNode(node cluster.Node) {
	t.nodes[node.NodeID] = node
	t.available[node.NodeID] = false // Start as unavailable until verified
}

// Ensure DefaultNodeAvailabilityTracker implements the NodeAvailabilityTracker
// interface.
var _ monitor.NodeAvailabilityTracker = (*DefaultNodeAvailabilityTracker)(nil)
