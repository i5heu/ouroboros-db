package keyValStore

import "log"

type StoreConfig struct {
	Paths            []string // absolute path at the moment only first path is supported
	minimumFreeSpace int      // in GB
}

type KeyValStore struct {
	config StoreConfig
}

func NewKeyValStore() *KeyValStore {
	return &KeyValStore{}
}

func (k *KeyValStore) Start(paths []string, minimumFreeSpace int) {
	k.config = StoreConfig{
		Paths:            paths,
		minimumFreeSpace: minimumFreeSpace,
	}

	err := k.checkConfig()
	if err != nil {
		log.Fatal(err)
	}

	// print the space left and allocated from the db
	k.PrintSpaceLeftAndAllocatedFromDB()

	// start the key value store

}

func (k *KeyValStore) checkConfig() error {
	// check if the paths exist
	// check if the paths are writeable
	// check if the paths have enough free space
	return nil
}

func (k *KeyValStore) PrintSpaceLeftAndAllocatedFromDB() {
	displayDiskUsage(k.config.Paths)
}
