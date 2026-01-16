// Package index defines interfaces for indexing operations in OuroborosDB.
package index

import (
	"context"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cluster"
)

// Index provides local indexing for vertices and their relationships.
type Index interface {
	// ParentChildIndex returns the parent-child relationship index.
	ParentChildIndex() ParentChildIndex

	// VersionIndex returns the version tracking index.
	VersionIndex() VersionIndex

	// KeyToHashIndex returns the key-to-hash lookup index.
	KeyToHashIndex() KeyToHashIndex
}

// ParentChildIndex manages parent-child relationships between vertices.
type ParentChildIndex interface {
	// AddChild adds a child hash under a parent.
	AddChild(ctx context.Context, parentHash, childHash hash.Hash) error

	// GetChildren returns all children of a parent.
	GetChildren(ctx context.Context, parentHash hash.Hash) ([]hash.Hash, error)

	// GetParent returns the parent of a child.
	GetParent(ctx context.Context, childHash hash.Hash) (hash.Hash, error)

	// RemoveChild removes a child from a parent.
	RemoveChild(ctx context.Context, parentHash, childHash hash.Hash) error
}

// VersionIndex tracks version vectors for conflict resolution.
type VersionIndex interface {
	// GetVersionHeads returns the current version vector heads.
	GetVersionHeads(ctx context.Context, rootHash hash.Hash) ([]hash.Hash, error)

	// UpdateVersionHead updates the version head for a root.
	UpdateVersionHead(ctx context.Context, rootHash, newHead hash.Hash) error
}

// KeyToHashIndex maps user-defined keys to vertex hashes.
type KeyToHashIndex interface {
	// SetKey associates a key with a vertex hash.
	SetKey(ctx context.Context, key string, vertexHash hash.Hash) error

	// GetHash retrieves the vertex hash for a key.
	GetHash(ctx context.Context, key string) (hash.Hash, error)

	// DeleteKey removes a key mapping.
	DeleteKey(ctx context.Context, key string) error

	// ListKeys returns all keys with a given prefix.
	ListKeys(ctx context.Context, prefix string) ([]string, error)
}

// DistributedIndex coordinates indexing across the cluster.
type DistributedIndex interface {
	// HashToNode returns the mapping for hash-to-node lookups.
	HashToNode() HashToNode

	// KeyToHashAndNode returns the mapping for key lookups.
	KeyToHashAndNode() KeyToHashAndNode
}

// HashToNode maps content hashes to their storage nodes.
type HashToNode interface {
	// GetNodeForHash returns the node responsible for a given hash.
	GetNodeForHash(ctx context.Context, h hash.Hash) (cluster.Node, error)

	// GetNodesForHash returns all nodes that have a copy of the hash (for replicas).
	GetNodesForHash(ctx context.Context, h hash.Hash) ([]cluster.Node, error)
}

// KeyToHashAndNode resolves keys to their hash and storage location.
type KeyToHashAndNode interface {
	// GetHashAndNodeForKey returns the hash and node for a given key.
	GetHashAndNodeForKey(ctx context.Context, key string) (hash.Hash, cluster.Node, error)
}
