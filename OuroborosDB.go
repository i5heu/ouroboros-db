package OuroborosDB

import (
	"OuroborosDB/internal/keyValStore"
	"OuroborosDB/internal/storage"
)

type OuroborosDB struct {
	ss *storage.Service
}

func NewOuroborosDB() *OuroborosDB {

	kvStore := keyValStore.NewKeyValStore()
	ss := storage.NewStorageService(kvStore)

	return &OuroborosDB{ss: &ss}
}

func (ou *OuroborosDB) Close() {
	ou.ss.Close()
}
