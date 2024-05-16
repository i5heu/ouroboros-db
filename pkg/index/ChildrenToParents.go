package index

import "github.com/i5heu/ouroboros-db/pkg/types"

func (i *Index) RebuildChildrenToParents(allEvents []types.Event) error {
	i.evChildToParentLock.Lock()
	defer i.evChildToParentLock.Unlock()

	// Clear the existing mapping
	clear(i.evChildToParent)

	// Rebuild the mapping
	for _, event := range allEvents {
		i.evChildToParent[event.EventHash] = event.HashOfParentEvent
	}

	return nil
}

func (i *Index) GetParentHashOfEvent(eventHash [64]byte) ([64]byte, bool) {
	i.evChildToParentLock.RLock()
	defer i.evChildToParentLock.RUnlock()
	parentHash, exists := i.evChildToParent[eventHash]
	return parentHash, exists
}

func (i *Index) GetDirectParentOfEvent(eventHash [64]byte) (*types.Event, error) {
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
