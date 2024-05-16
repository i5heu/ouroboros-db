package index

import (
	"sync"

	"github.com/i5heu/ouroboros-db/pkg/storage"
)

type Index struct {
	s                   storage.StorageService
	evParentToChild     map[[64]byte][][64]byte
	evParentToChildLock sync.RWMutex
	evChildToParent     map[[64]byte][64]byte
	evChildToParentLock sync.RWMutex
}

func NewIndex(ss storage.StorageService) *Index {

	i := &Index{
		s:                   ss,
		evParentToChildLock: sync.RWMutex{},
		evParentToChild:     make(map[[64]byte][][64]byte),
		evChildToParentLock: sync.RWMutex{},
		evChildToParent:     make(map[[64]byte][64]byte),
	}

	return i
}

func (i *Index) RebuildIndex() (uint64, error) {
	// get every event
	// TODO we might need to optimize memory usage here
	events, err := i.s.GetAllEvents()
	if err != nil {
		return 0, err
	}

	i.RebuildParentsToChildren(events)
	i.RebuildChildrenToParents(events)

	return uint64(len(events)), nil
}
