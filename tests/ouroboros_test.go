package ouroboros_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/internal/storage"
)

type setupDBConfig struct {
	totalEvents          int
	eventLevels          int // define how many levels of events to create
	returnAllEvents      bool
	returnRandomEvents   int // define how many random events to return
	notBuildIndex        bool
	generateWithFiles    bool
	generateWithMetaData bool
}

func randomString(n int) string {
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

func setupDBWithData(t testing.TB, conf setupDBConfig) (ou *ouroboros.OuroborosDB, backEventHashes [][64]byte) {
	if conf.totalEvents == 0 {
		conf.totalEvents = 1000
	}
	if conf.eventLevels < 1 {
		conf.eventLevels = 3
	}

	ou, err := ouroboros.Newouroboros(ouroboros.Config{
		Paths:                     []string{t.TempDir(), t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10,
	})
	if err != nil {
		t.Errorf("Newouroboros failed with error: %v", err)
	}

	if conf.totalEvents == 1 {
		rootEv, err := ou.DB.CreateRootEvent(time.Now().String())
		if err != nil {
			t.Errorf("CreateRootEvent failed with error: %v", err)
		}
		backEventHashes = [][64]byte{rootEv.EventHash}
		return ou, backEventHashes
	}

	// Create root events
	roots := make([]storage.Event, 0)
	numRoots := conf.totalEvents / (1 << (conf.eventLevels - 1))
	for i := 0; i < numRoots; i++ {
		rootEv, err := ou.DB.CreateRootEvent(time.Now().String())
		if err != nil {
			t.Errorf("CreateRootEvent failed with error: %v", err)
		}
		roots = append(roots, rootEv)
	}

	// Create event tree
	events := append([]storage.Event{}, roots...)
	for level := 1; level < conf.eventLevels; level++ {
		levelEvents := make([]storage.Event, 0)
		for _, parent := range events {
			for j := 0; j < 2; j++ {
				if len(events)+len(levelEvents) >= conf.totalEvents {
					break
				}

				if conf.generateWithFiles || conf.generateWithMetaData {
					ev, err := ou.DB.StoreFile(storage.StoreFileOptions{
						EventToAppendTo: parent,
						Metadata:        []byte(randomString(100)),
						File:            []byte(randomString(100)),
					})
					if err != nil {
						t.Errorf("CreateNewEvent failed with error: %v", err)
					}
					levelEvents = append(levelEvents, ev)

				} else {
					ev, err := ou.DB.CreateNewEvent(storage.EventOptions{
						HashOfParentEvent: parent.EventHash,
					})
					if err != nil {
						t.Errorf("CreateNewEvent failed with error: %v", err)
					}
					levelEvents = append(levelEvents, ev)
				}
			}
			if len(events)+len(levelEvents) >= conf.totalEvents {
				break
			}
		}
		events = append(events, levelEvents...)
		if len(events) >= conf.totalEvents {
			break
		}
	}

	// Return all events or random events
	if conf.returnAllEvents {
		backEventHashes = make([][64]byte, len(events))
		for i, ev := range events {
			backEventHashes[i] = ev.EventHash
		}
	} else {
		numRandom := conf.returnRandomEvents
		if numRandom == 0 || numRandom > len(events) {
			numRandom = len(events) / 2
		}
		backEventHashes = make([][64]byte, numRandom)
		for i := 0; i < numRandom; i++ {
			backEventHashes[i] = events[rand.Intn(len(events))].EventHash
		}
	}

	// Build index
	if !conf.notBuildIndex {
		_, err := ou.Index.RebuildIndex()
		if err != nil {
			t.Errorf("RebuildIndex failed with error: %v", err)
		}
	}

	return ou, backEventHashes
}

func Test_Index_RebuildIndex(t *testing.T) {
	testRows := 10000
	ou, _ := setupDBWithData(t, setupDBConfig{
		totalEvents:   testRows,
		notBuildIndex: true,
	})

	// Rebuild the index
	indexedRows, err := ou.Index.RebuildIndex()
	if err != nil {
		t.Errorf("RebuildIndex failed with error: %v", err)
	}

	if indexedRows != uint64(testRows) {
		t.Errorf("RebuildIndex failed, expected %d, got %d", testRows, indexedRows)
	}
}

func Test_Index_GetDirectChildrenOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:        100,
		returnRandomEvents: 10,
	})

	var childrenCount int // todo this is not a good test

	for _, evHash := range evs {
		children, err := ou.Index.GetDirectChildrenOfEvent(evHash)
		if err != nil {
			t.Errorf("GetDirectChildrenOfEvent failed with error: %v", err)
		}
		childrenCount += len(children)
	}
}

func Test_Index_GetChildrenHashesOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:        1000,
		returnRandomEvents: 100,
		eventLevels:        5,
	})

	var childrenCount int // todo this is not a good test

	for _, evHash := range evs {
		children := ou.Index.GetChildrenHashesOfEvent(evHash)
		childrenCount += len(children)
	}

	if childrenCount == 0 {
		t.Errorf("GetChildrenHashesOfEvent failed, expected non-zero count, got %d", childrenCount)
	}
}

func Test_DB_StoreFile(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:        100,
		returnRandomEvents: 10,
	})

	// todo this is not a good test

	for _, evHash := range evs {
		_, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: storage.Event{EventHash: evHash},
			Metadata:        []byte(randomString(100)),
			File:            []byte(randomString(100)),
		})
		if err != nil {
			t.Errorf("StoreFile failed with error: %v", err)
		}
	}
}

func Test_DB_GetFile(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents: 1,
	})

	// create some files
	rootEv := evs[0]
	testData := []struct {
		metadata []byte
		file     []byte
	}{
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
	}

	type storageEvent struct {
		ev       storage.Event
		metadata []byte
		file     []byte
	}

	storageEvnets := make([]storageEvent, 0)

	for _, data := range testData {
		sev, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: storage.Event{EventHash: rootEv},
			Metadata:        data.metadata,
			File:            data.file,
		})
		if err != nil {
			t.Errorf("StoreFile failed with error: %v", err)
		}

		if sev.ContentHashes == nil || len(sev.ContentHashes) == 0 {
			t.Errorf("StoreFile failed, expected non-nil, got nil")
		}

		storageEvnets = append(storageEvnets, storageEvent{
			ev:       sev,
			metadata: data.metadata,
			file:     data.file,
		})
	}

	for _, storEvent := range storageEvnets {
		file, err := ou.DB.GetFile(storEvent.ev)
		if err != nil {
			t.Errorf("GetFile failed with error: %v", err)
		}
		if len(file) == 0 {
			t.Errorf("GetFile failed, expected non-zero length, got %d", len(file))
		}

		if string(file) != string(storEvent.file) {
			t.Errorf("GetFile failed, expected %s, got %s", storEvent.file, file)
		}
	}
}

func Test_DB_GetEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
	})

	for _, evHash := range evs {
		ev, err := ou.DB.GetEvent(evHash)
		if err != nil {
			t.Errorf("GetEvent failed with error: %v", err)
		}
		if ev.EventHash == [64]byte{} {
			t.Errorf("GetEvent failed, expected non-zero hash, got %v", ev.EventHash)
		}
	}
}

func Test_DB_GetMetadata(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents: 1,
	})

	// create some files
	rootEv := evs[0]
	testData := []struct {
		metadata []byte
		file     []byte
	}{
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
		{[]byte(randomString(100)), []byte(randomString(100))},
	}

	type storageEvent struct {
		ev       storage.Event
		metadata []byte
		file     []byte
	}

	storageEvnets := make([]storageEvent, 0)

	for _, data := range testData {
		sev, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: storage.Event{EventHash: rootEv},
			Metadata:        data.metadata,
			File:            data.file,
		})
		if err != nil {
			t.Errorf("StoreFile failed with error: %v", err)
		}

		if sev.MetadataHashes == nil || len(sev.MetadataHashes) == 0 {
			t.Errorf("StoreFile failed, expected non-nil, got nil")
		}

		storageEvnets = append(storageEvnets, storageEvent{
			ev:       sev,
			metadata: data.metadata,
			file:     data.file,
		})
	}

	for _, storEvent := range storageEvnets {
		metadata, err := ou.DB.GetMetadata(storEvent.ev)
		if err != nil {
			t.Errorf("GetMetadata failed with error: %v", err)
		}
		if len(metadata) == 0 {
			t.Errorf("GetMetadata failed, expected non-zero length, got %d", len(metadata))
		}

		if string(metadata) != string(storEvent.metadata) {
			t.Errorf("GetMetadata failed, expected %s, got %s", storEvent.metadata, metadata)
		}
	}
}

func Test_DB_GetAllRootEvents(t *testing.T) {
	ou, _ := setupDBWithData(t, setupDBConfig{
		totalEvents: 10,
	})

	rootEvs, err := ou.DB.GetAllRootEvents()
	if err != nil {
		t.Errorf("GetAllRootEvents failed with error: %v", err)
	}

	fmt.Println(len(rootEvs))
	if len(rootEvs) == 0 {
		t.Errorf("GetAllRootEvents failed, expected non-zero count, got %d", len(rootEvs))
	}
}

func Test_DB_GetRootIndex(t *testing.T) {
	ou, _ := setupDBWithData(t, setupDBConfig{
		totalEvents: 10,
	})

	rootIndex, err := ou.DB.GetRootIndex()
	if err != nil {
		t.Errorf("GetRootIndex failed with error: %v", err)
	}

	if len(rootIndex) == 0 {
		t.Errorf("GetRootIndex failed, expected non-zero count, got %d", len(rootIndex))
	}
}

func Test_DB_GetRootEventsWithTitle(t *testing.T) {
	ou, _ := setupDBWithData(t, setupDBConfig{
		totalEvents: 10,
	})

	ou.DB.CreateRootEvent("test")

	rootEvs, err := ou.DB.GetRootEventsWithTitle("test")
	if err != nil {
		t.Errorf("GetRootEventsWithTitle failed with error: %v", err)
	}

	if len(rootEvs) == 0 {
		t.Errorf("GetRootEventsWithTitle failed, expected non-zero count, got %d", len(rootEvs))
	}
}

func Test_DB_CreateRootEvent(t *testing.T) {
	ou, _ := setupDBWithData(t, setupDBConfig{
		totalEvents: 10,
	})

	_, err := ou.DB.CreateRootEvent("test")
	if err != nil {
		t.Errorf("CreateRootEvent failed with error: %v", err)
	}
}

func Test_DB_CreateNewEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents: 1,
	})

	_, err := ou.DB.CreateNewEvent(storage.EventOptions{
		HashOfParentEvent: evs[0],
	})
	if err != nil {
		t.Errorf("CreateNewEvent failed with error: %v", err)
	}
}
