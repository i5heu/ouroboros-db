// Package distribution implements block distribution tracking for OuroborosDB.
package distribution

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
	"github.com/i5heu/ouroboros-db/pkg/distribution"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/i5heu/ouroboros-db/pkg/wal"
)

// OfflineNodeSyncService handles catchup for nodes that were offline.
//
// When a node comes online after being offline, it needs to:
// 1. Discover what blocks were distributed while it was offline
// 2. Request metadata for those blocks
// 3. Clear any WAL entries that are now in distributed blocks
//
// This prevents duplicate data and ensures consistency across the cluster.
type OfflineNodeSyncService struct {
	carrier carrier.Carrier
	tracker distribution.BlockDistributionTracker
	wal     wal.DistributedWAL

	// lastSyncTimestamp is the last time we successfully synced.
	// Used to request only blocks distributed since then.
	lastSyncTimestamp int64
	mu                sync.RWMutex

	// syncInterval controls how often background sync runs.
	syncInterval time.Duration
}

// NewOfflineNodeSyncService creates a new sync service.
func NewOfflineNodeSyncService(
	c carrier.Carrier,
	t distribution.BlockDistributionTracker,
	w wal.DistributedWAL,
) *OfflineNodeSyncService {
	return &OfflineNodeSyncService{
		carrier:           c,
		tracker:           t,
		wal:               w,
		lastSyncTimestamp: 0,
		syncInterval:      30 * time.Second,
	}
}

// SetLastSyncTimestamp sets the last sync timestamp.
// This should be called after loading persisted state on startup.
func (s *OfflineNodeSyncService) SetLastSyncTimestamp(ts int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncTimestamp = ts
}

// GetLastSyncTimestamp returns the last sync timestamp.
func (s *OfflineNodeSyncService) GetLastSyncTimestamp() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncTimestamp
}

// SyncMissedBlocks requests and processes blocks missed while offline.
//
// This is the main entry point for offline node catchup:
// 1. Get list of known nodes from carrier
// 2. Request missed block announcements from a peer
// 3. For each missed block, get its metadata
// 4. Clear any WAL entries that are now in distributed blocks
// 5. Update lastSyncTimestamp
//
// Returns the number of blocks synced and any error encountered.
func (s *OfflineNodeSyncService) SyncMissedBlocks(ctx context.Context) (int, error) {
	s.mu.Lock()
	since := s.lastSyncTimestamp
	s.mu.Unlock()

	// Get list of known nodes
	nodes, err := s.carrier.GetNodes(ctx)
	if err != nil {
		return 0, fmt.Errorf("get nodes: %w", err)
	}

	if len(nodes) == 0 {
		// No nodes to sync from, nothing to do
		return 0, nil
	}

	// Try to get missed blocks from available nodes
	var missedBlocks []hash.Hash
	var syncErr error

	for _, node := range nodes {
		missedBlocks, syncErr = s.requestMissedBlocks(ctx, node, since)
		if syncErr == nil {
			break
		}
		// Try next node if this one failed
	}

	if syncErr != nil {
		return 0, fmt.Errorf("request missed blocks: %w", syncErr)
	}

	// Process each missed block
	blocksProcessed := 0
	for _, blockHash := range missedBlocks {
		if err := s.processBlockForSync(ctx, nodes, blockHash); err != nil {
			// Log error but continue with other blocks
			continue
		}
		blocksProcessed++
	}

	// Update last sync timestamp
	s.mu.Lock()
	s.lastSyncTimestamp = time.Now().UnixMilli()
	s.mu.Unlock()

	return blocksProcessed, nil
}

// requestMissedBlocks asks a peer for blocks distributed since a timestamp.
func (s *OfflineNodeSyncService) requestMissedBlocks(
	ctx context.Context,
	node cluster.Node,
	since int64,
) ([]hash.Hash, error) {
	// In a full implementation, this would:
	// 1. Send MessageTypeMissedBlocksRequest to the node
	// 2. Wait for MessageTypeMissedBlocksResponse
	// 3. Deserialize and return the block hashes
	//
	// For now, we query our local tracker as a placeholder.
	// The actual network request/response will be integrated
	// when the Carrier handlers are fully implemented.
	return s.tracker.GetDistributedBlocksSince(ctx, since)
}

// processBlockForSync processes a single block for sync.
func (s *OfflineNodeSyncService) processBlockForSync(
	ctx context.Context,
	nodes []cluster.Node,
	blockHash hash.Hash,
) error {
	// Get block metadata
	metadata, err := s.requestBlockMetadata(ctx, nodes, blockHash)
	if err != nil {
		return fmt.Errorf("get block metadata: %w", err)
	}

	// Clear WAL entries that are now in this distributed block
	if err := s.clearStaleWALEntries(ctx, metadata); err != nil {
		return fmt.Errorf("clear stale WAL entries: %w", err)
	}

	return nil
}

// requestBlockMetadata asks peers for a block's metadata.
func (s *OfflineNodeSyncService) requestBlockMetadata(
	ctx context.Context,
	nodes []cluster.Node,
	blockHash hash.Hash,
) (*model.BlockMetadata, error) {
	// In a full implementation, this would:
	// 1. Send MessageTypeBlockMetadataRequest to nodes
	// 2. Wait for MessageTypeBlockMetadataResponse
	// 3. Return the metadata
	//
	// For now, we query our local tracker as a placeholder.
	return s.tracker.GetBlockMetadata(ctx, blockHash)
}

// clearStaleWALEntries removes WAL entries for data in a distributed block.
//
// When a block is distributed, any local WAL entries for the same data
// are stale and should be cleared. This matches WAL keys based on the
// block metadata's chunk/vertex/key entry information.
func (s *OfflineNodeSyncService) clearStaleWALEntries(
	ctx context.Context,
	metadata *model.BlockMetadata,
) error {
	var keysToDelete [][]byte

	// Build keys for chunk entries
	for _, chunkHash := range metadata.ChunkHashes {
		key := []byte("wal:chunk:" + chunkHash.String())
		keysToDelete = append(keysToDelete, key)
	}

	// Build keys for vertex entries
	for _, vertexHash := range metadata.VertexHashes {
		key := []byte("wal:vertex:" + vertexHash.String())
		keysToDelete = append(keysToDelete, key)
	}

	// Build keys for key entries (already in WAL key format)
	for _, keyStr := range metadata.KeyEntryKeys {
		keysToDelete = append(keysToDelete, []byte(keyStr))
	}

	if len(keysToDelete) == 0 {
		return nil
	}

	// Clear matching WAL entries
	return s.wal.ClearBlock(ctx, keysToDelete)
}

// StartBackgroundSync starts a background goroutine that periodically syncs.
//
// This is useful for nodes that may have intermittent connectivity.
// The goroutine will sync missed blocks at regular intervals.
//
// Cancel the context to stop the background sync.
func (s *OfflineNodeSyncService) StartBackgroundSync(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(s.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = s.SyncMissedBlocks(ctx)
			}
		}
	}()
}
