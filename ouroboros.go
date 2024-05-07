package ouroboros

import (
	"fmt"
	"os"
	"time"

	"github.com/i5heu/ouroboros-db/internal/index"
	"github.com/i5heu/ouroboros-db/internal/keyValStore"
	"github.com/i5heu/ouroboros-db/internal/storage"
	"github.com/i5heu/ouroboros-db/pkg/api"

	"github.com/sirupsen/logrus"

	"github.com/dgraph-io/badger/v4"
)

var log *logrus.Logger

type OuroborosDB struct {
	ss     *storage.Storage
	DB     api.DB
	Index  api.Index
	config Config
	log    *logrus.Logger
}

type Config struct {
	Paths                     []string
	MinimumFreeGB             int
	GarbageCollectionInterval time.Duration // in minutes
	Logger                    *logrus.Logger
}

func initializeLogger() *logrus.Logger {
	var log = logrus.New()

	logFile, err := os.OpenFile("log.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(fmt.Errorf("could not open log file: %v", err))
	}
	log.SetOutput(logFile)
	return log
}

func Newouroboros(conf Config) (*OuroborosDB, error) {
	if conf.Logger == nil {
		conf.Logger = initializeLogger()
		log = conf.Logger
	} else {
		log = conf.Logger
	}

	kvStore, err := keyValStore.NewKeyValStore(
		keyValStore.StoreConfig{
			Paths:            conf.Paths,
			MinimumFreeSpace: conf.MinimumFreeGB,
			Logger:           conf.Logger,
		})
	if err != nil {
		return nil, fmt.Errorf("error creating KeyValStore: %w", err)
	}

	ss := storage.CreateStorage(kvStore)
	index := index.NewIndex(ss)

	ou := &OuroborosDB{
		ss:     ss,
		DB:     ss,
		Index:  index,
		config: conf,
		log:    conf.Logger,
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
