package index

import (
	"OuroborosDB/internal/storage"
	"sync"
)

type Index struct {
	ss                  *storage.Storage
	evParentToChild     map[[64]byte][][64]byte
	evParentToChildLock sync.RWMutex
}

func NewIndex(ss *storage.Storage) *Index {

	i := &Index{
		ss:                  ss,
		evParentToChildLock: sync.RWMutex{},
		evParentToChild:     make(map[[64]byte][][64]byte),
	}

	return i
}

func (i *Index) RebuildIndex() (uint64, error) {
	// get every event
	events, err := i.ss.GetAllEvents()
	if err != nil {
		return 0, err
	}

	i.evParentToChildLock.Lock()
	defer i.evParentToChildLock.Unlock()

	// clear the map
	clear(i.evParentToChild)

	// create a map of parent to children
	for _, event := range events {
		i.evParentToChild[event.HashOfParentEvent] = append(i.evParentToChild[event.HashOfParentEvent], event.EventHash)
	}

	return uint64(len(events)), nil
}

func (i *Index) GetChildrenHashesOfEvent(event storage.Event) [][64]byte {
	i.evParentToChildLock.RLock()
	defer i.evParentToChildLock.RUnlock()
	return i.evParentToChild[event.EventHash]
}

func (i *Index) GetChildrenOfEvent(event storage.Event) ([]storage.Event, error) {
	childrenHashes := i.GetChildrenHashesOfEvent(event)
	children := make([]storage.Event, 0)

	for _, childHash := range childrenHashes {

		key := storage.GenerateKeyFromPrefixAndHash("Event:", childHash)
		child, err := i.ss.GetEvent(key)
		if err != nil {
			return nil, err
		}

		children = append(children, child)
	}

	return children, nil
}
