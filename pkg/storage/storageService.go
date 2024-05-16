package storage

import "github.com/i5heu/ouroboros-db/pkg/types"

// StorageService defines the methods for the storage service
type StorageService interface {
	CreateRootEvent(title string) (types.Event, error)
	GetAllRootEvents() ([]types.Event, error)
	GetRootIndex() ([]types.RootEventsIndex, error)
	GetRootEventsWithTitle(title string) ([]types.Event, error)
	CreateNewEvent(options EventOptions) (types.Event, error)
	GetEvent(hashOfEvent [64]byte) (types.Event, error)
	StoreFile(options StoreFileOptions) (types.Event, error)
	GetFile(eventOfFile types.Event) ([]byte, error)
	GetMetadata(eventOfFile types.Event) ([]byte, error)
	GarbageCollection() error
	GetAllEvents() ([]types.Event, error)
	Close()
}
