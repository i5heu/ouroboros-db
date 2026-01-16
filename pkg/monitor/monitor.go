// Package monitor defines interfaces for cluster monitoring in OuroborosDB.
package monitor

import (
	"context"

	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// ClusterMonitor monitors the health and status of the cluster.
type ClusterMonitor interface {
	// MonitorNodeHealth checks the health of all nodes in the cluster.
	MonitorNodeHealth(ctx context.Context) error

	// GetNodeStatus returns the current status of a specific node.
	GetNodeStatus(ctx context.Context, nodeID cluster.NodeID) (NodeStatus, error)

	// GetClusterHealth returns the overall health of the cluster.
	GetClusterHealth(ctx context.Context) (ClusterHealth, error)

	// RegisterHealthCallback registers a callback for health status changes.
	RegisterHealthCallback(callback HealthCallback)
}

// NodeAvailabilityTracker tracks which nodes are available and responsive.
type NodeAvailabilityTracker interface {
	// TrackAvailability starts tracking availability for all known nodes.
	TrackAvailability(ctx context.Context) error

	// IsNodeAvailable returns whether a node is currently available.
	IsNodeAvailable(nodeID cluster.NodeID) bool

	// GetAvailableNodes returns all currently available nodes.
	GetAvailableNodes() []cluster.Node

	// GetUnavailableNodes returns all currently unavailable nodes.
	GetUnavailableNodes() []cluster.Node

	// MarkNodeUnavailable manually marks a node as unavailable.
	MarkNodeUnavailable(nodeID cluster.NodeID)

	// MarkNodeAvailable manually marks a node as available.
	MarkNodeAvailable(nodeID cluster.NodeID)
}

// NodeStatus represents the current status of a node.
type NodeStatus struct {
	// NodeID identifies the node.
	NodeID cluster.NodeID

	// Available indicates if the node is reachable.
	Available bool

	// LastSeen is the Unix timestamp of the last successful communication.
	LastSeen int64

	// Latency is the last measured latency in milliseconds.
	Latency int64

	// Load is the node's current load (0.0 - 1.0).
	Load float64

	// DiskUsage is the percentage of disk space used.
	DiskUsage float64

	// MemoryUsage is the percentage of memory used.
	MemoryUsage float64
}

// ClusterHealth represents the overall health of the cluster.
type ClusterHealth struct {
	// Healthy indicates if the cluster is in a healthy state.
	Healthy bool

	// TotalNodes is the total number of nodes in the cluster.
	TotalNodes int

	// AvailableNodes is the number of currently available nodes.
	AvailableNodes int

	// UnavailableNodes is the number of currently unavailable nodes.
	UnavailableNodes int

	// ReplicationFactor is the current effective replication factor.
	ReplicationFactor int
}

// HealthCallback is called when the health status of a node changes.
type HealthCallback func(nodeID cluster.NodeID, oldStatus, newStatus NodeStatus)
