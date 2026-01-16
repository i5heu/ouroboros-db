// Package datarouter defines interfaces for routing data operations across the cluster.
//
// The DataRouter is responsible for coordinating data distribution and retrieval
// in a distributed OuroborosDB cluster. It works with the DistributedIndex to
// locate data and with the Carrier to communicate with remote nodes.
//
// # Architecture Position
//
//	┌────────────────────────────────────────────────────────────┐
//	│                           CAS                              │
//	│              (high-level content operations)               │
//	├────────────────────────────────────────────────────────────┤
//	│                       DataRouter                           │
//	│    (coordinates distribution and retrieval across nodes)   │
//	├─────────────────┬──────────────────┬───────────────────────┤
//	│ DistributedIndex│     Carrier      │      BlockStore       │
//	│  (find nodes)   │  (communicate)   │  (local storage)      │
//	└─────────────────┴──────────────────┴───────────────────────┘
//
// # Distribution Strategy
//
// When storing data, the DataRouter:
//  1. Receives a Block from the WAL (via CAS)
//  2. Splits the Block into BlockSlices (Reed-Solomon encoding)
//  3. Consults DistributedIndex for target nodes
//  4. Sends slices to nodes via Carrier
//  5. Tracks successful distributions
//
// # Retrieval Strategy
//
// When retrieving data, the DataRouter:
//  1. Consults DistributedIndex to find nodes storing the data
//  2. Requests slices from nodes via Carrier
//  3. Collects enough slices for reconstruction (k minimum)
//  4. Reconstructs the Block using Reed-Solomon decoding
//  5. Returns the complete Block (or Vertex)
//
// # Redundancy and Fault Tolerance
//
// The DataRouter ensures data availability through:
//   - Reed-Solomon encoding: tolerate p node failures
//   - Replica placement: spread slices across failure domains
//   - Automatic repair: detect and fix under-replication
package datarouter

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// DataRouter routes data operations across the cluster.
//
// DataRouter is the coordination layer between CAS and the distributed storage
// infrastructure. It handles the complexity of distributed data placement,
// retrieval, and reconstruction.
//
// # Thread Safety
//
// Implementations must be safe for concurrent use. Multiple goroutines may
// store and retrieve data simultaneously.
//
// # Error Handling
//
// Methods return errors for:
//   - Insufficient available nodes for distribution
//   - Network failures when communicating with nodes
//   - Insufficient slices for reconstruction
//   - Data corruption during retrieval
type DataRouter interface {
	// StoreVertex stores a vertex and returns its hash.
	//
	// In a distributed deployment, this may involve:
	//  1. Determining target nodes via consistent hashing
	//  2. Sending the vertex to target nodes
	//  3. Waiting for acknowledgment from a quorum
	//
	// Parameters:
	//   - vertex: The vertex to store
	//
	// Returns:
	//   - The vertex hash (for future retrieval)
	//   - Error if storage fails
	//
	// Note: Vertices are typically stored as part of Blocks, not individually.
	// This method may buffer the vertex until a Block is sealed.
	StoreVertex(ctx context.Context, vertex model.Vertex) (hash.Hash, error)

	// RetrieveVertex retrieves a vertex by its hash.
	//
	// This may involve:
	//  1. Querying DistributedIndex for node locations
	//  2. Requesting the vertex from available nodes
	//  3. Returning the first successful response
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to retrieve
	//
	// Returns:
	//   - The vertex metadata
	//   - Error if the vertex cannot be found or retrieved
	RetrieveVertex(ctx context.Context, vertexHash hash.Hash) (model.Vertex, error)

	// DeleteVertex marks a vertex for deletion across the cluster.
	//
	// Deletion in a distributed system is eventually consistent:
	//  1. Log the deletion to the DeletionWAL
	//  2. Propagate deletion request to nodes storing the vertex
	//  3. Actual removal happens during garbage collection
	//
	// Parameters:
	//   - vertexHash: The hash of the vertex to delete
	//
	// Returns:
	//   - Error if the deletion cannot be initiated
	DeleteVertex(ctx context.Context, vertexHash hash.Hash) error

	// DistributeBlockSlices distributes block slices to appropriate nodes.
	//
	// This is the primary distribution method called after a Block is sealed.
	// The process:
	//  1. Split the Block into k data + p parity BlockSlices
	//  2. For each slice, determine target nodes (via consistent hashing)
	//  3. Send slices to their target nodes via Carrier
	//  4. Wait for acknowledgment from required nodes
	//  5. Return success when distribution is complete
	//
	// Parameters:
	//   - block: The complete block to distribute
	//
	// Returns:
	//   - Error if distribution fails (e.g., insufficient nodes available)
	//
	// The number of nodes required depends on the replication factor and
	// Reed-Solomon parameters (k+p slices, possibly replicated).
	DistributeBlockSlices(ctx context.Context, block model.Block) error

	// RetrieveBlock retrieves a block, potentially reconstructing from slices.
	//
	// This handles the complexity of distributed block retrieval:
	//  1. Query DistributedIndex for nodes storing slices of this block
	//  2. Request slices from available nodes (in parallel)
	//  3. Collect at least k slices (data or parity)
	//  4. Reconstruct the block using Reed-Solomon decoding
	//  5. Verify the block hash matches
	//
	// Parameters:
	//   - blockHash: The hash of the block to retrieve
	//
	// Returns:
	//   - The reconstructed block
	//   - Error if insufficient slices or reconstruction fails
	//
	// This method is resilient to node failures: as long as k slices
	// are available, the block can be reconstructed.
	RetrieveBlock(ctx context.Context, blockHash hash.Hash) (model.Block, error)
}
