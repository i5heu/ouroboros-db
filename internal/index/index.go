// Package index provides indexing implementations for OuroborosDB.
package index

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/index"
)

// LocalIndex implements the Index interface for local indexing.
type LocalIndex struct {
	parentChild *parentChildIndex
	version     *versionIndex
	keyToHash   *keyToHashIndex
}

// NewLocalIndex creates a new LocalIndex instance.
func NewLocalIndex() *LocalIndex {
	return &LocalIndex{
		parentChild: newParentChildIndex(),
		version:     newVersionIndex(),
		keyToHash:   newKeyToHashIndex(),
	}
}

// ParentChildIndex returns the parent-child relationship index.
func (i *LocalIndex) ParentChildIndex() index.ParentChildIndex {
	return i.parentChild
}

// VersionIndex returns the version tracking index.
func (i *LocalIndex) VersionIndex() index.VersionIndex {
	return i.version
}

// KeyToHashIndex returns the key-to-hash lookup index.
func (i *LocalIndex) KeyToHashIndex() index.KeyToHashIndex {
	return i.keyToHash
}

// Ensure LocalIndex implements the Index interface.
var _ index.Index = (*LocalIndex)(nil)

// parentChildIndex implements ParentChildIndex interface.
type parentChildIndex struct {
	mu               sync.RWMutex
	parentToChildren map[hash.Hash][]hash.Hash
	childToParent    map[hash.Hash]hash.Hash
}

func newParentChildIndex() *parentChildIndex {
	return &parentChildIndex{
		parentToChildren: make(map[hash.Hash][]hash.Hash),
		childToParent:    make(map[hash.Hash]hash.Hash),
	}
}

func (p *parentChildIndex) AddChild(
	ctx context.Context,
	parentHash, childHash hash.Hash,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.parentToChildren[parentHash] = append(
		p.parentToChildren[parentHash],
		childHash,
	)
	p.childToParent[childHash] = parentHash
	return nil
}

func (p *parentChildIndex) GetChildren(
	ctx context.Context,
	parentHash hash.Hash,
) ([]hash.Hash, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	children, exists := p.parentToChildren[parentHash]
	if !exists {
		return nil, nil
	}
	// Return a copy to prevent external modification
	result := make([]hash.Hash, len(children))
	copy(result, children)
	return result, nil
}

func (p *parentChildIndex) GetParent(
	ctx context.Context,
	childHash hash.Hash,
) (hash.Hash, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	parent, exists := p.childToParent[childHash]
	if !exists {
		return hash.Hash{}, fmt.Errorf(
			"index: parent not found for child %s",
			childHash,
		)
	}
	return parent, nil
}

func (p *parentChildIndex) RemoveChild(
	ctx context.Context,
	parentHash, childHash hash.Hash,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	children := p.parentToChildren[parentHash]
	for i, child := range children {
		if child == childHash {
			p.parentToChildren[parentHash] = append(children[:i], children[i+1:]...)
			break
		}
	}
	delete(p.childToParent, childHash)
	return nil
}

var _ index.ParentChildIndex = (*parentChildIndex)(nil)

// versionIndex implements VersionIndex interface.
type versionIndex struct {
	mu    sync.RWMutex
	heads map[hash.Hash][]hash.Hash
}

func newVersionIndex() *versionIndex {
	return &versionIndex{
		heads: make(map[hash.Hash][]hash.Hash),
	}
}

func (v *versionIndex) GetVersionHeads(
	ctx context.Context,
	rootHash hash.Hash,
) ([]hash.Hash, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	heads, exists := v.heads[rootHash]
	if !exists {
		return nil, nil
	}
	result := make([]hash.Hash, len(heads))
	copy(result, heads)
	return result, nil
}

func (v *versionIndex) UpdateVersionHead(
	ctx context.Context,
	rootHash, newHead hash.Hash,
) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.heads[rootHash] = []hash.Hash{newHead}
	return nil
}

var _ index.VersionIndex = (*versionIndex)(nil)

// keyToHashIndex implements KeyToHashIndex interface.
type keyToHashIndex struct {
	mu        sync.RWMutex
	keyToHash map[string]hash.Hash
}

func newKeyToHashIndex() *keyToHashIndex {
	return &keyToHashIndex{
		keyToHash: make(map[string]hash.Hash),
	}
}

func (k *keyToHashIndex) SetKey(
	ctx context.Context,
	key string,
	vertexHash hash.Hash,
) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	k.keyToHash[key] = vertexHash
	return nil
}

func (k *keyToHashIndex) GetHash(
	ctx context.Context,
	key string,
) (hash.Hash, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	h, exists := k.keyToHash[key]
	if !exists {
		return hash.Hash{}, fmt.Errorf("index: key %s not found", key)
	}
	return h, nil
}

func (k *keyToHashIndex) DeleteKey(ctx context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	delete(k.keyToHash, key)
	return nil
}

func (k *keyToHashIndex) ListKeys(
	ctx context.Context,
	prefix string,
) ([]string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	var keys []string
	for key := range k.keyToHash {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

var _ index.KeyToHashIndex = (*keyToHashIndex)(nil)
