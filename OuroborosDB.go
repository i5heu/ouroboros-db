package OuroborosDB

import (
	"OuroborosDB/internal/index"
	"OuroborosDB/internal/keyValStore"
	"OuroborosDB/internal/storage"
	"OuroborosDB/pkg/api"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dgraph-io/badger/v4"
)

type OuroborosDB struct {
	ss     *storage.Storage
	DB     api.DB
	Index  api.Index
	config Config
	log    *log.Logger
}

type Config struct {
	Paths                     []string
	MinimumFreeGB             int
	GarbageCollectionInterval time.Duration // in minutes
	Logger                    *log.Logger
}

func initializeLogger() *log.Logger {
	logFile, err := os.OpenFile("log.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(fmt.Errorf("could not open log file: %v", err))
	}
	log.SetOutput(logFile)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
	return log.New(logFile, "", log.LstdFlags)
}

func NewOuroborosDB(conf Config) (*OuroborosDB, error) {
	if conf.Logger == nil {
		conf.Logger = initializeLogger()
	} else {
		log.SetOutput(conf.Logger.Writer())
	}

	kvStore, err := keyValStore.NewKeyValStore(
		keyValStore.StoreConfig{
			Paths:            conf.Paths,
			MinimumFreeSpace: conf.MinimumFreeGB,
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
