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
	fastMetaToEvent     map[string][]types.Hash
	fastMetaToEventLock sync.RWMutex
}

func NewIndex(ss storage.StorageService) *Index {
	return &Index{
		s:               ss,
		evParentToChild: make(map[types.Hash][]types.Hash),
		evChildToParent: make(map[types.Hash]types.Hash),
		fastMetaToEvent: make(map[string][]types.Hash),
	}
}

func (i *Index) RebuildIndex() (uint64, error) {
	events, err := i.s.GetAllEvents()
	if err != nil {
		return 0, err
	}

	i.RebuildParentsToChildren(events)
	i.RebuildChildrenToParents(events)
	i.RebuildFastMeta(events)

	return uint64(len(events)), nil
}
