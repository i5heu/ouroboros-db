// Package index defines interfaces for indexing operations in OuroborosDB.
//
// This package provides indexing abstractions for both local and distributed
// operations. Indexes enable efficient lookups and traversals that would
// otherwise require scanning all stored data.
//
// # Index Architecture
//
// The indexing system has two layers:
//
//  1. Local Index: Per-node indexes for fast local lookups
//     - ParentChildIndex: DAG structure (parent → children, child → parent)
//     - VersionIndex: Version vector heads for conflict resolution
//     - KeyToHashIndex: User-defined key → vertex hash mappings
//
//  2. Distributed Index: Cluster-wide coordination
//     - HashToNode: Maps content hashes to storage nodes
//     - KeyToHashAndNode: Resolves keys to hash and location
//
// # DAG Navigation
//
// The ParentChildIndex enables efficient DAG traversal:
//
//	                    [root]
//	                   /      \
//	            [child1]      [child2]
//	           /    \              \
//	      [gc1]    [gc2]          [gc3]
//
// Traversal operations:
//   - GetChildren(root) → [child1, child2]
//   - GetParent(gc1) → child1
//   - GetParent(root) → zero hash (root has no parent)
//
// # Version Control
//
// The VersionIndex tracks version history for conflict resolution:
//
//	[root] → version heads: [v1, v2, v3]  (concurrent modifications)
//
// When multiple nodes modify the same data, version heads diverge.
// Conflict resolution merges these heads into a single new version.
//
// # Key Mapping
//
// The KeyToHashIndex provides human-readable access to content:
//
//	"/users/alice/profile" → vertex hash
//	"/config/settings" → vertex hash
//
// Keys support hierarchical organization via prefix queries.
package index

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Index provides local indexing for vertices and their relationships.
//
// Index is the facade for all local indexing operations. It provides access
// to specialized sub-indexes for different query patterns.
//
// # Usage
//
//	idx := node.Index()
//	children, _ := idx.ParentChildIndex().GetChildren(ctx, rootHash)
//	heads, _ := idx.VersionIndex().GetVersionHeads(ctx, rootHash)
//	vertexHash, _ := idx.KeyToHashIndex().GetHash(ctx, "/my/key")
//
// # Thread Safety
//
// All sub-indexes are safe for concurrent use from multiple goroutines.
type Index interface {
	// ParentChildIndex returns the parent-child relationship index.
	//
	// Use this for DAG traversal operations (finding children, finding parent).
	ParentChildIndex() ParentChildIndex

	// VersionIndex returns the version tracking index.
	//
	// Use this for version control operations (finding heads, updating versions).
	VersionIndex() VersionIndex

	// KeyToHashIndex returns the key-to-hash lookup index.
	//
	// Use this for named/keyed access to vertices.
	KeyToHashIndex() KeyToHashIndex
}

// ParentChildIndex manages parent-child relationships between vertices.
//
// This index maintains bidirectional mappings that enable efficient DAG
// traversal in both directions:
//
//   - Parent → Children: "What vertices are children of X?"
//   - Child → Parent: "What is the parent of vertex Y?"
//
// # Consistency
//
// AddChild and RemoveChild maintain both directions atomically.
// The index is updated when vertices are stored via CAS.
//
// # Root Vertices
//
// Root vertices (those with zero parent hash) have no parent entry.
// GetParent returns the zero hash for root vertices.
type ParentChildIndex interface {
	// AddChild adds a child hash under a parent.
	//
	// This updates both the parent→children and child→parent mappings.
	// If the relationship already exists, this is a no-op.
	//
	// Parameters:
	//   - parentHash: The parent vertex hash
	//   - childHash: The child vertex hash to add
	//
	// Returns:
	//   - Error if the index update fails
	AddChild(ctx context.Context, parentHash, childHash hash.Hash) error

	// GetChildren returns all children of a parent.
	//
	// Parameters:
	//   - parentHash: The parent vertex hash
	//
	// Returns:
	//   - Slice of child vertex hashes (may be empty)
	//   - Error if the lookup fails
	//
	// The returned order is not guaranteed.
	GetChildren(ctx context.Context, parentHash hash.Hash) ([]hash.Hash, error)

	// GetParent returns the parent of a child.
	//
	// Parameters:
	//   - childHash: The child vertex hash
	//
	// Returns:
	//   - Parent vertex hash (zero hash if root or not found)
	//   - Error if the lookup fails
	GetParent(ctx context.Context, childHash hash.Hash) (hash.Hash, error)

	// RemoveChild removes a child from a parent.
	//
	// This updates both the parent→children and child→parent mappings.
	// If the relationship doesn't exist, this is a no-op.
	//
	// Parameters:
	//   - parentHash: The parent vertex hash
	//   - childHash: The child vertex hash to remove
	//
	// Returns:
	//   - Error if the index update fails
	RemoveChild(ctx context.Context, parentHash, childHash hash.Hash) error
}

// VersionIndex tracks version vectors for conflict resolution.
//
// In a distributed system, concurrent modifications create version branches.
// The VersionIndex tracks the "heads" (latest versions) for each root vertex,
// enabling conflict detection and resolution.
//
// # Version Heads
//
// A root with one head is consistent (linear history).
// A root with multiple heads has diverged (needs conflict resolution).
//
// Example:
//
//	Initial:  [root] → heads: [v1]
//	User A:   [root] → heads: [v2a]  (A modified v1 → v2a)
//	User B:   [root] → heads: [v2b]  (B modified v1 → v2b, concurrently)
//	Merged:   [root] → heads: [v2a, v2b]  (conflict detected)
//	Resolved: [root] → heads: [v3]  (merged into single version)
type VersionIndex interface {
	// GetVersionHeads returns the current version vector heads.
	//
	// Parameters:
	//   - rootHash: The root vertex hash to query
	//
	// Returns:
	//   - Slice of head vertex hashes (usually 1, multiple indicates conflict)
	//   - Error if the lookup fails
	//
	// An empty slice indicates no versions exist for this root.
	GetVersionHeads(ctx context.Context, rootHash hash.Hash) ([]hash.Hash, error)

	// UpdateVersionHead updates the version head for a root.
	//
	// This is called when a new version is created. In case of concurrent
	// updates from multiple nodes, this may result in multiple heads.
	//
	// Parameters:
	//   - rootHash: The root vertex hash
	//   - newHead: The new version head
	//
	// Returns:
	//   - Error if the update fails
	UpdateVersionHead(ctx context.Context, rootHash, newHead hash.Hash) error
}

// KeyToHashIndex maps user-defined keys to vertex hashes.
//
// This index provides human-readable, hierarchical access to content.
// Keys are strings that can represent paths, identifiers, or any naming
// scheme meaningful to the application.
//
// # Key Format
//
// Keys are arbitrary strings, but path-like formats are recommended:
//   - "/users/alice/profile"
//   - "/config/database/connection"
//   - "project:settings:v2"
//
// # Prefix Queries
//
// ListKeys supports prefix-based queries for hierarchical navigation:
//
//	ListKeys(ctx, "/users/") → ["/users/alice", "/users/bob", ...]
type KeyToHashIndex interface {
	// SetKey associates a key with a vertex hash.
	//
	// If the key already exists, its mapping is updated.
	//
	// Parameters:
	//   - key: The string key (should not be empty)
	//   - vertexHash: The vertex hash to associate
	//
	// Returns:
	//   - Error if the mapping cannot be stored
	SetKey(ctx context.Context, key string, vertexHash hash.Hash) error

	// GetHash retrieves the vertex hash for a key.
	//
	// Parameters:
	//   - key: The string key to look up
	//
	// Returns:
	//   - The associated vertex hash
	//   - Error if the key doesn't exist or lookup fails
	GetHash(ctx context.Context, key string) (hash.Hash, error)

	// DeleteKey removes a key mapping.
	//
	// If the key doesn't exist, this is a no-op (no error).
	//
	// Parameters:
	//   - key: The string key to remove
	//
	// Returns:
	//   - Error if the deletion fails
	DeleteKey(ctx context.Context, key string) error

	// ListKeys returns all keys with a given prefix.
	//
	// Parameters:
	//   - prefix: The prefix to match (use "" for all keys)
	//
	// Returns:
	//   - Slice of matching keys
	//   - Error if the query fails
	//
	// Example:
	//   ListKeys(ctx, "/users/") → ["/users/alice", "/users/bob"]
	//   ListKeys(ctx, "") → all keys
	ListKeys(ctx context.Context, prefix string) ([]string, error)
}

// DistributedIndex coordinates indexing across the cluster.
//
// While the local Index handles per-node data, DistributedIndex provides
// cluster-wide lookups to locate data across all nodes.
//
// # Data Location
//
// In a distributed deployment, data is sharded across nodes. DistributedIndex
// maintains the mapping from content hashes to node locations:
//
//	hash(content) → consistent hash → responsible node(s)
//
// # Usage with DataRouter
//
// DistributedIndex is typically used by the DataRouter to determine where
// to send store/retrieve requests:
//
//	node := distIndex.HashToNode().GetNodeForHash(ctx, hash)
//	carrier.SendMessageToNode(ctx, node.NodeID, request)
type DistributedIndex interface {
	// HashToNode returns the mapping for hash-to-node lookups.
	//
	// Use this to find which node(s) store a specific piece of content.
	HashToNode() HashToNode

	// KeyToHashAndNode returns the mapping for key lookups.
	//
	// Use this to resolve a key to both its hash and storage location.
	KeyToHashAndNode() KeyToHashAndNode
}

// HashToNode maps content hashes to their storage nodes.
//
// This interface provides the core distributed hash table functionality,
// mapping content hashes to the nodes responsible for storing them.
//
// # Replication
//
// Content may be replicated across multiple nodes for redundancy.
// GetNodeForHash returns the primary node, while GetNodesForHash returns
// all nodes with replicas.
type HashToNode interface {
	// GetNodeForHash returns the primary node responsible for a given hash.
	//
	// Parameters:
	//   - h: The content hash to locate
	//
	// Returns:
	//   - The primary storage node
	//   - Error if the lookup fails or no node is responsible
	GetNodeForHash(ctx context.Context, h hash.Hash) (cluster.Node, error)

	// GetNodesForHash returns all nodes that have a copy of the hash.
	//
	// This includes the primary node and all replica nodes.
	//
	// Parameters:
	//   - h: The content hash to locate
	//
	// Returns:
	//   - Slice of nodes storing replicas (primary first, then replicas)
	//   - Error if the lookup fails
	GetNodesForHash(ctx context.Context, h hash.Hash) ([]cluster.Node, error)
}

// KeyToHashAndNode resolves keys to their hash and storage location.
//
// This combines KeyToHashIndex lookup with node location in a single
// operation, useful for distributed key-based access.
type KeyToHashAndNode interface {
	// GetHashAndNodeForKey returns the hash and primary node for a given key.
	//
	// This is equivalent to:
	//   hash := keyIndex.GetHash(key)
	//   node := hashToNode.GetNodeForHash(hash)
	//
	// But may be more efficient as a single distributed operation.
	//
	// Parameters:
	//   - key: The string key to resolve
	//
	// Returns:
	//   - The vertex hash
	//   - The primary storage node
	//   - Error if the key doesn't exist or lookup fails
	GetHashAndNodeForKey(ctx context.Context, key string) (hash.Hash, cluster.Node, error)
}
