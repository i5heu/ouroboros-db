package OuroborosDB

import (
	"OuroborosDB/internal/keyValStore"
	"OuroborosDB/internal/storage"
	"OuroborosDB/pkg/api"
	"fmt"
	"log"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type OuroborosDB struct {
	ss     *storage.Storage
	DB     api.DB
	config Config
}

type Config struct {
	Paths                     []string
	MinimumFreeGB             int
	GarbageCollectionInterval time.Duration // in minutes
}

func NewOuroborosDB(conf Config) (*OuroborosDB, error) {
	kvStore, err := keyValStore.NewKeyValStore(
		keyValStore.StoreConfig{
			Paths:            conf.Paths,
			MinimumFreeSpace: conf.MinimumFreeGB,
		})
	if err != nil {
		return nil, fmt.Errorf("error creating KeyValStore: %w", err)
	}

	ss := storage.CreateStorage(kvStore)

	ou := &OuroborosDB{
		ss:     &ss,
		DB:     &ss,
		config: conf,
	}

	go ou.createGarbageCollection()

	return ou, nil
}

func (ou *OuroborosDB) Close() {
	ou.ss.Close()
}

func (ou *OuroborosDB) createGarbageCollection() {
	ticker := time.NewTicker(ou.config.GarbageCollectionInterval * time.Minute)
	for range ticker.C {
		err := ou.ss.GarbageCollection()
		fmt.Println("Garbage Collection", badger.ErrNoRewrite)
		if err != nil {
			log.Fatal("Error during garbage collection: ", err) // maybe we have to rework this here a bit
		}
	}
}
