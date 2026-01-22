// Package rebalance defines interfaces for data rebalancing in OuroborosDB.
package rebalance

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// DataReBalancer manages data distribution across the cluster.
// It ensures data is properly balanced when nodes join or leave.
type DataReBalancer interface {
	// BalanceData initiates a rebalancing operation.
	BalanceData(ctx context.Context) error

	// GetRebalanceStatus returns the current rebalancing status.
	GetRebalanceStatus(ctx context.Context) (RebalanceStatus, error)

	// CancelRebalance cancels an ongoing rebalance operation.
	CancelRebalance(ctx context.Context) error
}

// ReplicationMonitoring monitors the replication status of data.
type ReplicationMonitoring interface {
	// MonitorReplications checks that all data has the required replication
	// level.
	MonitorReplications(ctx context.Context) error

	// GetUnderReplicatedBlocks returns blocks that don't meet the replication
	// target.
	GetUnderReplicatedBlocks(ctx context.Context) ([]hash.Hash, error)

	// GetOverReplicatedBlocks returns blocks that exceed the replication target.
	GetOverReplicatedBlocks(ctx context.Context) ([]hash.Hash, error)

	// RepairReplication repairs the replication of a specific block.
	RepairReplication(ctx context.Context, blockHash hash.Hash) error
}

// SyncIndexTree handles synchronization of index trees across nodes.
type SyncIndexTree interface {
	// Sync synchronizes the local index tree with remote nodes.
	Sync(ctx context.Context) error

	// GetSyncStatus returns the current synchronization status.
	GetSyncStatus(ctx context.Context) (SyncStatus, error)
}

// RebalanceStatus represents the status of a rebalancing operation.
type RebalanceStatus struct {
	// InProgress indicates if rebalancing is currently happening.
	InProgress bool

	// Progress is the percentage complete (0-100).
	Progress float64

	// BlocksMoved is the number of blocks that have been moved.
	BlocksMoved int64

	// BlocksRemaining is the number of blocks still to be moved.
	BlocksRemaining int64

	// BytesMoved is the total bytes transferred.
	BytesMoved int64

	// StartTime is when the rebalance started (Unix timestamp).
	StartTime int64

	// EstimatedCompletion is the estimated completion time (Unix timestamp).
	EstimatedCompletion int64
}

// SyncStatus represents the status of index synchronization.
type SyncStatus struct {
	// Synchronized indicates if the index is fully synchronized.
	Synchronized bool

	// LastSync is the Unix timestamp of the last successful sync.
	LastSync int64

	// PendingUpdates is the number of updates waiting to be synced.
	PendingUpdates int64
}
