package index

import (
	"fmt"

	"github.com/i5heu/ouroboros-db/pkg/types"
)

func (i *Index) RebuildFastMeta(allEvents []types.Event) (uint64, error) {
	i.fastMetaToEventLock.Lock()
	defer i.fastMetaToEventLock.Unlock()

	clear(i.fastMetaToEvent)

	for _, event := range allEvents {
		for _, fm := range event.FastMeta {
			fmString := fm.String()
			i.fastMetaToEvent[fmString] = append(i.fastMetaToEvent[fmString], event.EventIdentifier.EventHash)
		}
	}

	return uint64(len(i.fastMetaToEvent)), nil
}

func (i *Index) GetEventHashesByFastMetaParameter(fmp types.FastMetaParameter) []types.Hash {
	i.fastMetaToEventLock.RLock()
	defer i.fastMetaToEventLock.RUnlock()

	return i.fastMetaToEvent[fmp.String()]
}

// will return all events that have all given fast meta parameters
func (i *Index) GetEventHashesByFastMeta(fm types.FastMeta) []types.Hash {
	i.fastMetaToEventLock.RLock()
	defer i.fastMetaToEventLock.RUnlock()

	parameterHashes := make([][]types.Hash, 0)

	for _, fmp := range fm {
		evHashes := i.GetEventHashesByFastMetaParameter(fmp)
		parameterHashes = append(parameterHashes, evHashes)
	}

	eventHashes := intersect(parameterHashes...)

	return eventHashes
}

// TODO we can maybe optimize this by using go routines
func intersect(slices ...[]types.Hash) []types.Hash {
	if len(slices) == 0 {
		return nil
	}

	// Initialize the intersection with the first slice
	intersection := make(map[[64]byte]struct{})
	for _, item := range slices[0] {
		intersection[item] = struct{}{}
	}

	// Iterate through the rest of the slices and update the intersection
	for i := 1; i < len(slices); i++ {
		currentSet := make(map[[64]byte]struct{})
		for _, item := range slices[i] {
			if _, found := intersection[item]; found {
				currentSet[item] = struct{}{}
			}
		}
		intersection = currentSet
	}

	// Convert the map back to a slice
	result := make([]types.Hash, 0, len(intersection))
	for item := range intersection {
		result = append(result, item)
	}

	return result
}

func (i *Index) GetEventsByFastMeta(fm types.FastMeta) ([]types.Event, error) {
	i.fastMetaToEventLock.RLock()
	defer i.fastMetaToEventLock.RUnlock()

	evHashes := i.GetEventHashesByFastMeta(fm)

	events := make([]types.Event, 0)
	for _, eventHash := range evHashes {
		event, err := i.s.GetEvent(eventHash)
		if err != nil {
			return nil, fmt.Errorf("could not get event with hash %s: %w - maybe you need to rebuild the index", eventHash, err)
		}
		events = append(events, event)
	}

	return events, nil
}
