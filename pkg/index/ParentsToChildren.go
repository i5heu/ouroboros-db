package index

import "github.com/i5heu/ouroboros-db/pkg/types"

func (i *Index) RebuildParentsToChildren(allEvents []types.Event) error {
	i.evParentToChildLock.Lock()
	defer i.evParentToChildLock.Unlock()

	clear(i.evParentToChild)

	for _, event := range allEvents {
		i.evParentToChild[event.HashOfParentEvent] = append(i.evParentToChild[event.HashOfParentEvent], event.EventHash)
	}

	return nil
}

func (i *Index) GetChildrenHashesOfEvent(eventHash [64]byte) [][64]byte {
	i.evParentToChildLock.RLock()
	defer i.evParentToChildLock.RUnlock()
	return i.evParentToChild[eventHash]
}

func (i *Index) GetDirectChildrenOfEvent(eventHash [64]byte) ([]types.Event, error) {
	childrenHashes := i.GetChildrenHashesOfEvent(eventHash)
	children := make([]types.Event, 0)

	for _, childHash := range childrenHashes {

		child, err := i.s.GetEvent(childHash)
		if err != nil {
			return nil, err
		}

		children = append(children, child)
	}

	return children, nil
}
