package OuroborosDB_test

import (
	"OuroborosDB"
	"OuroborosDB/internal/storage"
	"fmt"
	"math/rand"
	"testing"
	"time"
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

func setupDBWithData(t testing.TB, conf setupDBConfig) (ou *OuroborosDB.OuroborosDB, backEventHashes [][64]byte) {
	if conf.totalEvents == 0 {
		conf.totalEvents = 1000
	}
	if conf.eventLevels < 1 {
		conf.eventLevels = 3
	}

	ou, err := OuroborosDB.NewOuroborosDB(OuroborosDB.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10,
	})
	if err != nil {
		t.Errorf("NewOuroborosDB failed with error: %v", err)
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
		_, err = ou.Index.RebuildIndex()
		if err != nil {
			t.Errorf("RebuildIndex failed with error: %v", err)
		}
	}

	return ou, backEventHashes
}

func Test_Index_RebuildIndex(t *testing.T) {
	testEvs := 1000
	ou, _ := setupDBWithData(t, setupDBConfig{})

	// Rebuild the index
	indexedRows, err := ou.Index.RebuildIndex()
	if err != nil {
		t.Errorf("RebuildIndex failed with error: %v", err)
	}

	if indexedRows != uint64(testEvs) {
		t.Errorf("RebuildIndex failed, expected %d, got %d", testEvs, indexedRows)
	}
}

func Benchmark_Index_RebuildingIndex(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents: 10000,
	})

	b.Run("RebuildIndex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.Index.RebuildIndex()
			if err != nil {
				b.Errorf("RebuildIndex failed with error: %v", err)
			}
		}
	})

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

func Benchmark_Index_GetDirectChildrenOfEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:        10000,
		returnRandomEvents: 1000,
	})

	b.Run("GetChildrenOfEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.Index.GetDirectChildrenOfEvent(evs[rand.Intn(len(evs))])
			if err != nil {
				b.Errorf("RebuildIndex failed with error: %v", err)
			}
		}
	})

}

func Test_Index_GetChildrenHashesOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:        100,
		returnRandomEvents: 10,
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

func Benchmark_Index_GetChildrenHashesOfEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:        10000,
		returnRandomEvents: 1000,
	})

	b.Run("GetChildrenHashesOfEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ou.Index.GetChildrenHashesOfEvent(evs[rand.Intn(len(evs))])
		}
	})

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

func Benchmark_DB_StoreFile(b *testing.B) {
	ou, evsHashes := setupDBWithData(b, setupDBConfig{
		totalEvents:        10000,
		returnRandomEvents: 1000,
	})

	ev := make([]storage.Event, len(evsHashes))
	for i, evHash := range evsHashes {
		ev[i], _ = ou.DB.GetEvent(evHash)
	}

	b.Run("StoreFile", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.StoreFile(storage.StoreFileOptions{
				EventToAppendTo: ev[rand.Intn(len(ev))],
				Metadata:        []byte(randomString(100)),
				File:            []byte(randomString(100)),
			})
			if err != nil {
				b.Errorf("StoreFile failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetFile(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
	})

	storeEvs := make([]storage.Event, 0)

	for ev := range evs {
		storeEv, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: storage.Event{EventHash: evs[ev]},
			Metadata:        []byte(randomString(100)),
			File:            []byte(randomString(100)),
		})
		if err != nil {
			b.Errorf("StoreFile failed with error: %v", err)
		}
		storeEvs = append(storeEvs, storeEv)

	}

	b.Run("GetFile", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetFile(storeEvs[rand.Intn(len(storeEvs))])
			if err != nil {
				b.Errorf("GetFile failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:     10000,
		returnAllEvents: true,
	})

	b.Run("GetEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetEvent(evs[rand.Intn(len(evs))])
			if err != nil {
				b.Errorf("GetEvent failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetMetadata(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:          10000,
		returnRandomEvents:   1000,
		generateWithMetaData: true,
	})

	storeEvs := make([]storage.Event, 0)

	for _, ev := range evs {
		storeEv, err := ou.DB.GetEvent(ev)
		if err != nil {
			b.Errorf("GetEvent failed with error: %v", err)
		}

		storeEvs = append(storeEvs, storeEv)
	}

	b.Run("GetMetadata", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetMetadata(storeEvs[rand.Intn(len(storeEvs))])
			if err != nil {
				b.Errorf("GetMetadata failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetAllRootEvents(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents: 10000,
	})

	b.Run("GetAllRootEvents", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetAllRootEvents()
			if err != nil {
				b.Errorf("GetAllRootEvents failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetRootIndex(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents: 10000,
	})

	b.Run("GetRootIndex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetRootIndex()
			if err != nil {
				b.Errorf("GetRootIndex failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_GetRootEventsWithTitle(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents: 10000,
	})

	var rootTitle []string
	for i := 0; i < 1000; i++ {
		rootTitle = append(rootTitle, fmt.Sprintf("test-%d", i))
		_, err := ou.DB.CreateRootEvent(rootTitle[i])
		if err != nil {
			b.Errorf("CreateRootEvent failed with error: %v", err)
		}
	}

	b.Run("GetRootEventsWithTitle", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.GetRootEventsWithTitle(rootTitle[rand.Intn(len(rootTitle))])
			if err != nil {
				b.Errorf("GetRootEventsWithTitle failed with error: %v", err)
			}
		}
	})
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

func Benchmark_DB_CreateRootEvent(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents: 1,
	})

	b.Run("CreateRootEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			eventName := fmt.Sprintf("test-%d-%s", i, time.Now().String())
			_, err := ou.DB.CreateRootEvent(eventName)
			if err != nil {
				b.Errorf("CreateRootEvent failed with error: %v. Name of the event: %s", err, eventName)
			}
		}
	})
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

func Benchmark_DB_CreateNewEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents: 1000,
	})

	b.Run("CreateNewEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.CreateNewEvent(storage.EventOptions{
				HashOfParentEvent: evs[rand.Intn(len(evs))],
			})
			if err != nil {
				b.Errorf("CreateNewEvent failed with error: %v", err)
			}
		}
	})
}
