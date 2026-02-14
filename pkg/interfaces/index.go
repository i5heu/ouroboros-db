package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Index interface { // A
	GetParentChildIndex() ParentChildIndex
	GetVersionIndex() VersionIndex
	GetKeyToHashIndex() KeyToHashIndex
}

type ParentChildIndex interface { // A
	GetChildren(parentHash hash.Hash) []hash.Hash
	GetParent(childHash hash.Hash) (hash.Hash, bool)
	AddRelation(parent, child hash.Hash)
	RemoveRelation(parent, child hash.Hash)
}

type VersionIndex interface { // A
	GetVersionVectorHeads(hash hash.Hash) []hash.Hash
	AddVersionHead(h, head hash.Hash)
	RemoveVersionHead(h, head hash.Hash)
}

type KeyToHashIndex interface { // A
	GetHashForKey(key string) (hash.Hash, bool)
	SetHashForKey(key string, h hash.Hash)
	RemoveKey(key string)
}

type DistributedIndex interface { // A
	GetHashToNode() HashToNode
	GetKeyToHashAndNode() KeyToHashAndNode
}

type HashToNode interface { // A
	GetNodeForHash(h hash.Hash) Node
}

type KeyToHashAndNode interface { // A
	GetHashAndNodeForKey(key string) (hash.Hash, Node, error)
}
