/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!

A embedded database built around the concept of event trees, emphasizing data deduplication and data integrity checks. By structuring data into event trees, OuroborosDB ensures efficient and intuitive data management. Key features include:

  - Data Deduplication: Eliminates redundant data through efficient chunking and hashing mechanisms.
  - Data Integrity Checks: Uses SHA-512 hashes to verify the integrity of stored data.
  - Event-Based Architecture: Organizes data hierarchically for easy retrieval and management.
  - Scalable Concurrent Processing: Optimized for concurrent processing to handle large-scale data.
  - Log Management and Indexing: Provides efficient logging and indexing for performance monitoring.
  - Non-Deletable Events: Once stored, events cannot be deleted or altered, ensuring the immutability and auditability of the data.
  - (To be implemented) Temporary Events: Allows the creation of temporary events that can be marked as temporary and safely cleaned up later for short-term data storage needs.

There are in the moment two main components in the database
  - [storage]: The storage component, which is responsible for storing the data on the disk.
  - [index]: The index component, which is responsible for creating indexes in RAM for faster acces.
*/
package ouroboros

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

type OuroborosDB struct {
	log    *logrus.Logger
	config Config
}

type Config struct {
	Paths         []string // Paths to store the data, currently only the first path is used
	MinimumFreeGB uint
	Logger        *logrus.Logger
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

// NewOuroborosDB creates a new OuroborosDB instance.
//   - It returns an error if the initialization fails.
//   - Use DB or Index to interact with the database.
//   - Index is only RAM based and will be lost after a restart.
//   - DB is persistent.
func NewOuroborosDB(conf Config) (*OuroborosDB, error) {
	if conf.Logger == nil {
		conf.Logger = initializeLogger()
		log = conf.Logger
	} else {
		log = conf.Logger
	}

	ou := &OuroborosDB{
		log:    conf.Logger,
		config: conf,
	}

	return ou, nil
}

// Close closes the database
// It is important to call this function before the program exits.
// Use it like this:
//
//	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{Paths: []string{"./data"}})
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer ou.Close()
func (ou *OuroborosDB) Close() {
	return
}
