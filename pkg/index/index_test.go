package index

import (
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/mocks"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestIndex_RebuildIndex(t *testing.T) {
	mockStorageService := new(mocks.StorageService)

	events := []types.Event{
		{EventIdentifier: types.EventIdentifier{EventHash: types.Hash{1}}, ParentEvent: types.Hash{0}},
		{EventIdentifier: types.EventIdentifier{EventHash: types.Hash{2}}, ParentEvent: types.Hash{1}},
	}

	mockStorageService.On("GetAllEvents").Return(events, nil)

	index := NewIndex(mockStorageService)

	count, err := index.RebuildIndex()

	assert.NoError(t, err)
	assert.Equal(t, uint64(2), count)
	mockStorageService.AssertExpectations(t)
}

func TestIndex_RebuildParentsToChildren(t *testing.T) {
	mockStorageService := new(mocks.StorageService)
	index := NewIndex(mockStorageService)

	parentHash := types.Hash{1}
	childHash1 := types.Hash{2}
	childHash2 := types.Hash{3}

	events := []types.Event{
		{EventIdentifier: types.EventIdentifier{EventHash: childHash1}, ParentEvent: parentHash},
		{EventIdentifier: types.EventIdentifier{EventHash: childHash2}, ParentEvent: parentHash},
	}

	err := index.RebuildParentsToChildren(events)
	assert.NoError(t, err)

	index.evParentToChildLock.RLock()
	defer index.evParentToChildLock.RUnlock()
	assert.Contains(t, index.evParentToChild[parentHash], childHash1)
	assert.Contains(t, index.evParentToChild[parentHash], childHash2)
}

func TestIndex_GetChildrenHashesOfEvent(t *testing.T) {
	index := &Index{
		evParentToChild: make(map[types.Hash][]types.Hash),
	}

	parentHash := types.Hash{1}
	childHash := types.Hash{2}
	index.evParentToChild[parentHash] = append(index.evParentToChild[parentHash], childHash)

	retrievedChildren := index.GetChildrenHashesOfEvent(parentHash)
	assert.Contains(t, retrievedChildren, childHash)
}

func TestIndex_GetDirectChildrenOfEvent(t *testing.T) {
	mockStorageService := new(mocks.StorageService)
	index := NewIndex(mockStorageService)

	parentHash := types.Hash{1}
	childHash := types.Hash{2}
	index.evParentToChild[parentHash] = append(index.evParentToChild[parentHash], childHash)

	childEvent := types.Event{EventIdentifier: types.EventIdentifier{EventHash: childHash}}
	mockStorageService.On("GetEvent", childHash).Return(childEvent, nil)

	retrievedChildren, err := index.GetDirectChildrenOfEvent(parentHash)
	assert.NoError(t, err)
	assert.Contains(t, retrievedChildren, childEvent)
	mockStorageService.AssertExpectations(t)
}

func TestIndex_RebuildChildrenToParents(t *testing.T) {
	mockStorageService := new(mocks.StorageService)
	index := NewIndex(mockStorageService)

	parentHash := types.Hash{1}
	childHash1 := types.Hash{2}
	childHash2 := types.Hash{3}

	events := []types.Event{
		{EventIdentifier: types.EventIdentifier{EventHash: childHash1}, ParentEvent: parentHash},
		{EventIdentifier: types.EventIdentifier{EventHash: childHash2}, ParentEvent: parentHash},
	}

	err := index.RebuildChildrenToParents(events)
	assert.NoError(t, err)

	index.evChildToParentLock.RLock()
	defer index.evChildToParentLock.RUnlock()
	assert.Equal(t, parentHash, index.evChildToParent[childHash1])
	assert.Equal(t, parentHash, index.evChildToParent[childHash2])
}

func TestIndex_GetParentHashOfEvent(t *testing.T) {
	index := &Index{
		evChildToParent: make(map[types.Hash]types.Hash),
	}

	parentHash := types.Hash{1}
	childHash := types.Hash{2}
	index.evChildToParent[childHash] = parentHash

	retrievedParentHash, exists := index.GetParentHashOfEvent(childHash)
	assert.True(t, exists)
	assert.Equal(t, parentHash, retrievedParentHash)
}

func TestIndex_GetDirectParentOfEvent(t *testing.T) {
	mockStorageService := new(mocks.StorageService)
	index := NewIndex(mockStorageService)

	parentHash := types.Hash{1}
	childHash := types.Hash{2}
	index.evChildToParent[childHash] = parentHash

	parentEvent := types.Event{EventIdentifier: types.EventIdentifier{EventHash: parentHash}}
	mockStorageService.On("GetEvent", parentHash).Return(parentEvent, nil)

	retrievedParentEvent, err := index.GetDirectParentOfEvent(childHash)
	assert.NoError(t, err)
	assert.Equal(t, &parentEvent, retrievedParentEvent)
	mockStorageService.AssertExpectations(t)
}
