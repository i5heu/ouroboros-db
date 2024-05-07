package api

import "github.com/i5heu/ouroboros-db/internal/storage"

type DB interface {
	StoreFile(options storage.StoreFileOptions) (storage.Event, error)
	GetFile(eventOfFile storage.Event) ([]byte, error)
	GetEvent(hashOfEvent [64]byte) (storage.Event, error)
	GetMetadata(eventOfFile storage.Event) ([]byte, error)
	GetAllRootEvents() ([]storage.Event, error)
	GetRootIndex() ([]storage.RootEventsIndex, error)
	GetRootEventsWithTitle(title string) ([]storage.Event, error)
	CreateRootEvent(title string) (storage.Event, error)
	CreateNewEvent(options storage.EventOptions) (storage.Event, error)
}

type Index interface {
	RebuildIndex() (uint64, error)
	GetChildrenHashesOfEvent(eventHash [64]byte) [][64]byte
	GetDirectChildrenOfEvent(eventHash [64]byte) ([]storage.Event, error)
}
