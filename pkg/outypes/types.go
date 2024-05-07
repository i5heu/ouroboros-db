package outypes

import (
	"ouroboros-db/internal/storage"
)

// Event represents an event in the EventChain, the absolute top of a EventChain is a RootEvent, look at rootEvents.go
type Event = storage.Event
type EventOptions = storage.EventOptions
type RootEventsIndex = storage.RootEventsIndex
type StoreFileOptions = storage.StoreFileOptions
