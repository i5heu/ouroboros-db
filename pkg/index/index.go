package index

import (
	"sync"

	"github.com/i5heu/ouroboros-db/pkg/storage"
	"github.com/i5heu/ouroboros-db/pkg/types"
)

type Index struct {
	s                   storage.StorageService
	evParentToChild     map[types.Hash][]types.Hash
	evParentToChildLock sync.RWMutex
	evChildToParent     map[types.Hash]types.Hash
	evChildToParentLock sync.RWMutex
}

func NewIndex(ss storage.StorageService) *Index {
	return &Index{
		s:               ss,
		evParentToChild: make(map[types.Hash][]types.Hash),
		evChildToParent: make(map[types.Hash]types.Hash),
	}
}

func (i *Index) RebuildIndex() (uint64, error) {
	events, err := i.s.GetAllEvents()
	if err != nil {
		return 0, err
	}

	i.RebuildParentsToChildren(events)
	i.RebuildChildrenToParents(events)

	return uint64(len(events)), nil
}
