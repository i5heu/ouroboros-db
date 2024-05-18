package index

import "github.com/i5heu/ouroboros-db/pkg/types"

func (i *Index) RebuildChildrenToParents(allEvents []types.Event) error {
	i.evChildToParentLock.Lock()
	defer i.evChildToParentLock.Unlock()

	clear(i.evChildToParent)

	for _, event := range allEvents {
		i.evChildToParent[event.EventIdentifier.EventHash] = event.ParentEvent
	}

	return nil
}

func (i *Index) GetParentHashOfEvent(eventHash types.Hash) (types.Hash, bool) {
	i.evChildToParentLock.RLock()
	defer i.evChildToParentLock.RUnlock()
	parentHash, exists := i.evChildToParent[eventHash]
	return parentHash, exists
}

func (i *Index) GetDirectParentOfEvent(eventHash types.Hash) (*types.Event, error) {
	parentHash, exists := i.GetParentHashOfEvent(eventHash)
	if !exists {
		return nil, nil // No parent found
	}

	parentEvent, err := i.s.GetEvent(parentHash)
	if err != nil {
		return nil, err
	}

	return &parentEvent, nil
}
