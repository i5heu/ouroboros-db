package api

import "OuroborosDB/internal/storage"

type DB interface {
	StoreFile(options storage.StoreFileOptions) (storage.Event, error)
	GetFile(eventOfFile storage.Event) ([]byte, error)
	GetEvent(key []byte) (storage.Event, error)
	GetMetadata(eventOfFile storage.Event) ([]byte, error)
	GetAllRootEvents() ([]storage.Event, error)
	GetRootIndex() ([]storage.RootEventsIndex, error)
	GetRootEventsWithTitle(title string) ([]storage.Event, error)
	CreateRootEvent(title string) (storage.Event, error)
	CreateNewEvent(options storage.EventOptions) (storage.Event, error)
}

type Index interface {
	RebuildIndex() (uint64, error)
	GetChildrenHashesOfEvent(event storage.Event) [][64]byte
	GetChildrenOfEvent(event storage.Event) ([]storage.Event, error)
}
