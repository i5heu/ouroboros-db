// Package distribution provides interfaces and types for managing block distribution
// across the OuroborosDB cluster.
//
// The distribution system ensures that blocks are replicated to multiple nodes
// before WAL entries are cleared, providing durability guarantees even in the
// presence of node failures.
//
// # Distribution Flow
//
// 1. When WAL buffer is full, SealBlock() creates a Block and returns WAL keys
// 2. DataRouter calls tracker.StartDistribution() to begin tracking
// 3. Block slices are distributed to target nodes
// 4. As nodes confirm receipt, tracker.RecordSliceConfirmation() is called
// 5. Once MinNodesRequired confirm, the block is marked as Distributed
// 6. WAL entries (identified by WAL keys) can now be safely deleted
// 7. Block announcement is broadcast to all nodes
//
// # Offline Node Support
//
// Nodes that were offline can catch up by:
// 1. Requesting missed block announcements via GetDistributedBlocksSince()
// 2. Requesting block metadata via GetBlockMetadata()
// 3. Clearing stale WAL entries that match the metadata
package distribution

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// BlockDistributionTracker manages block distribution state and confirmations.
//
// The tracker is responsible for:
//   - Recording which blocks are being distributed
//   - Tracking node confirmations for each block's slices
//   - Determining when a block has sufficient confirmations
//   - Providing block metadata for offline node sync
//
// Implementations must be safe for concurrent use and must persist state
// to survive restarts.
type BlockDistributionTracker interface {
	// StartDistribution begins tracking a new block distribution.
	//
	// This should be called immediately after SealBlock() returns, before
	// any slices are sent to nodes. The walKeys parameter contains the
	// BadgerDB keys that should be deleted once distribution is confirmed.
	//
	// Parameters:
	//   - block: The block being distributed
	//   - walKeys: The WAL keys to delete after successful distribution
	//
	// Returns:
	//   - The distribution record for tracking state
	//   - Error if tracking cannot be initiated
	StartDistribution(
		ctx context.Context,
		block model.Block,
		walKeys [][]byte,
	) (*model.BlockDistributionRecord, error)

	// RecordSliceConfirmation records that a node confirmed receipt of a slice.
	//
	// This is called when a MessageTypeBlockSliceAck is received. The method
	// updates the confirmation count and determines if the distribution
	// threshold has been reached.
	//
	// Parameters:
	//   - blockHash: The block that the slice belongs to
	//   - sliceHash: The specific slice that was confirmed
	//   - nodeID: The node that confirmed receipt
	//
	// Returns:
	//   - distributed: true if this confirmation caused the block to reach
	//     the distribution threshold (UniqueNodesConfirmed >= MinNodesRequired)
	//   - Error if the confirmation cannot be recorded
	RecordSliceConfirmation(
		ctx context.Context,
		blockHash hash.Hash,
		sliceHash hash.Hash,
		nodeID string,
	) (distributed bool, err error)

	// GetDistributionState returns the current state of a block's distribution.
	//
	// This can be used to check on the progress of a distribution or to
	// retrieve the WAL keys for a distributed block.
	//
	// Returns:
	//   - The distribution record, or nil if not found
	//   - Error if the state cannot be retrieved
	GetDistributionState(
		ctx context.Context,
		blockHash hash.Hash,
	) (*model.BlockDistributionRecord, error)

	// GetPendingDistributions returns all blocks in Pending or Distributing state.
	//
	// This is used on startup to resume any distributions that were
	// interrupted by a crash or restart.
	//
	// Returns:
	//   - Slice of distribution records for incomplete distributions
	//   - Error if the records cannot be retrieved
	GetPendingDistributions(ctx context.Context) ([]*model.BlockDistributionRecord, error)

	// MarkDistributed explicitly marks a block as successfully distributed.
	//
	// This is typically called automatically when RecordSliceConfirmation
	// reaches the threshold, but can be called manually if needed.
	//
	// Returns:
	//   - Error if the state cannot be updated
	MarkDistributed(ctx context.Context, blockHash hash.Hash) error

	// MarkFailed marks a block distribution as failed.
	//
	// This is called when distribution cannot be completed after maximum
	// retries. The WAL entries are NOT deleted, allowing for manual
	// intervention or future retry.
	//
	// Parameters:
	//   - blockHash: The block that failed to distribute
	//   - reason: A description of why distribution failed
	//
	// Returns:
	//   - Error if the state cannot be updated
	MarkFailed(ctx context.Context, blockHash hash.Hash, reason string) error

	// GetDistributedBlocksSince returns block hashes distributed after a timestamp.
	//
	// This is used by nodes coming online to discover blocks they missed
	// while offline. The timestamp is in Unix milliseconds.
	//
	// Parameters:
	//   - since: Unix milliseconds timestamp; returns blocks after this time
	//
	// Returns:
	//   - Slice of block hashes that were distributed after the timestamp
	//   - Error if the query fails
	GetDistributedBlocksSince(ctx context.Context, since int64) ([]hash.Hash, error)

	// GetBlockMetadata returns metadata for a distributed block.
	//
	// This is used by nodes to determine which WAL entries can be cleared.
	// The metadata contains chunk hashes, vertex hashes, and key entry keys
	// that correspond to WAL entries.
	//
	// Parameters:
	//   - blockHash: The block to get metadata for
	//
	// Returns:
	//   - The block metadata, or nil if not found
	//   - Error if the metadata cannot be retrieved
	GetBlockMetadata(ctx context.Context, blockHash hash.Hash) (*model.BlockMetadata, error)
}
