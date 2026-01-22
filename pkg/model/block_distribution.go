// Package model provides data structures for OuroborosDB.
package model

import (
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// BlockDistributionState represents the current distribution status of a block.
type BlockDistributionState uint8

const (
	// BlockStatePending indicates the block is created but not yet being
	// distributed.
	BlockStatePending BlockDistributionState = iota
	// BlockStateDistributing indicates the block is actively being distributed to
	// nodes.
	BlockStateDistributing
	// BlockStateDistributed indicates the block has been confirmed on sufficient
	// nodes.
	BlockStateDistributed
	// BlockStateFailed indicates distribution failed after maximum retries.
	BlockStateFailed
)

// String returns a human-readable representation of the distribution state.
func (s BlockDistributionState) String() string {
	switch s {
	case BlockStatePending:
		return "Pending"
	case BlockStateDistributing:
		return "Distributing"
	case BlockStateDistributed:
		return "Distributed"
	case BlockStateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// BlockDistributionRecord tracks the distribution status of a single block.
// This record is persisted to ensure distribution state survives restarts.
type BlockDistributionRecord struct {
	// BlockHash uniquely identifies the block being distributed.
	BlockHash hash.Hash

	// State is the current distribution state.
	State BlockDistributionState

	// CreatedAt is when the distribution was initiated.
	CreatedAt time.Time

	// UpdatedAt is when the record was last modified.
	UpdatedAt time.Time

	// SliceConfirmations maps sliceHash to the list of nodeIDs that confirmed
	// receipt.
	SliceConfirmations map[hash.Hash][]string

	// TotalSlices is the total number of slices (k data + p parity).
	TotalSlices uint8

	// UniqueNodesConfirmed counts distinct nodes that have confirmed any slice.
	UniqueNodesConfirmed int

	// RetryCount tracks distribution retry attempts.
	RetryCount int

	// WALKeys stores the BadgerDB keys for WAL entries in this block.
	// These are deleted once distribution is confirmed.
	WALKeys [][]byte
}

// BlockMetadata contains block structure info for offline node sync.
// Nodes use this to determine which WAL entries can be cleared.
type BlockMetadata struct {
	// BlockHash uniquely identifies the block.
	BlockHash hash.Hash

	// ChunkHashes lists all chunk hashes contained in this block.
	ChunkHashes []hash.Hash

	// VertexHashes lists all vertex hashes contained in this block.
	VertexHashes []hash.Hash

	// KeyEntryKeys lists all key entry composite keys (for WAL matching).
	KeyEntryKeys []string

	// CreatedAt is the block creation timestamp (Unix milliseconds).
	CreatedAt int64
}

// DistributionConfig holds configuration for the distribution tracker.
type DistributionConfig struct {
	// MinNodesRequired is the minimum number of nodes that must confirm
	// receipt before a block is considered distributed. Default: 3.
	MinNodesRequired int

	// DistributionTimeout is how long to wait for confirmations before retrying.
	DistributionTimeout time.Duration

	// MaxRetries is the maximum number of retry attempts before marking as
	// failed.
	MaxRetries int

	// AnnounceToAll controls whether to broadcast announcements to all nodes.
	AnnounceToAll bool
}

// DefaultDistributionConfig returns a configuration with sensible defaults.
func DefaultDistributionConfig() DistributionConfig {
	return DistributionConfig{
		MinNodesRequired:    3,
		DistributionTimeout: 30 * time.Second,
		MaxRetries:          3,
		AnnounceToAll:       true,
	}
}
