/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"fmt"
	"os"
	"path/filepath"

	crypt "github.com/i5heu/ouroboros-crypt"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

type OuroborosDB struct {
	log    *logrus.Logger
	config Config
	crypt  *crypt.Crypt
	kv     *ouroboroskv.KV
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

	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}

	// Initialize ouroboros-crypt with key from Paths[0]/ouroboros.key
	cryptKeyPath := filepath.Join(conf.Paths[0], "ouroboros.key")
	cryptInstance, err := crypt.NewFromFile(cryptKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ouroboros-crypt: %v", err)
	}

	// Initialize ouroboros-kv with Paths[0]/kv
	kvConfig := &ouroboroskv.Config{
		Paths:            []string{filepath.Join(conf.Paths[0], "kv")},
		MinimumFreeSpace: int(conf.MinimumFreeGB),
		Logger:           conf.Logger,
	}
	kvInstance, err := ouroboroskv.Init(cryptInstance, kvConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ouroboros-kv: %v", err)
	}

	ou := &OuroborosDB{
		log:    conf.Logger,
		config: conf,
		crypt:  cryptInstance,
		kv:     kvInstance,
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
	if ou.kv != nil {
		if err := ou.kv.Close(); err != nil {
			ou.log.Errorf("failed to close KV store: %v", err)
		}
	}
}
