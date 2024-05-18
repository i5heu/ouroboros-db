package ouroboros_test

import (
	"fmt"
	"log"
	"math/rand"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/pkg/storage"
	"github.com/i5heu/ouroboros-db/pkg/types"
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

func setupDBWithData(t testing.TB, conf setupDBConfig) (ou *ouroboros.OuroborosDB, backEventHashes []types.Hash) {
	if conf.totalEvents == 0 {
		conf.totalEvents = 1000
	}
	if conf.eventLevels < 1 {
		conf.eventLevels = 3
	}

	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir(), t.TempDir()},
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
		backEventHashes = []types.Hash{rootEv.EventIdentifier.EventHash}
		return ou, backEventHashes
	}

	// Create root events
	roots := make([]types.Event, 0)
	numRoots := conf.totalEvents / (1 << (conf.eventLevels - 1))
	for i := 0; i < numRoots; i++ {
		rootEv, err := ou.DB.CreateRootEvent(time.Now().String())
		if err != nil {
			t.Errorf("CreateRootEvent failed with error: %v", err)
		}
		roots = append(roots, rootEv)
	}

	// Create event tree
	events := append([]types.Event{}, roots...)
	for level := 1; level < conf.eventLevels; level++ {
		levelEvents := make([]types.Event, 0)
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
						ParentEvent: parent.EventIdentifier.EventHash,
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
		backEventHashes = make([]types.Hash, len(events))
		for i, ev := range events {
			backEventHashes[i] = ev.EventIdentifier.EventHash
		}
	} else {
		numRandom := conf.returnRandomEvents
		if numRandom == 0 || numRandom > len(events) {
			numRandom = len(events) / 2
		}
		backEventHashes = make([]types.Hash, numRandom)
		for i := 0; i < numRandom; i++ {
			backEventHashes[i] = events[rand.Intn(len(events))].EventIdentifier.EventHash
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

func Test_Index_GetDirectParentOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
		eventLevels:     5,
	})

	var parentCount int

	for _, evHash := range evs {
		parent, err := ou.Index.GetDirectParentOfEvent(evHash)
		if err != nil {
			t.Errorf("GetDirectParentOfEvent failed with error: %v", err)
		}
		if parent == nil {
			continue
		}
		parentCount++
	}

	if parentCount == 0 {
		t.Errorf("GetDirectParentOfEvent failed, expected non-zero count, got %d", parentCount)
	}
}

func Test_Index_GetParentHashOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
		eventLevels:     5,
	})

	var childrenCount int

	for _, evHash := range evs {
		parent := ou.Index.GetParentHashOfEvent(evHash)
		if parent == [64]byte{} {
			continue
		}
		childrenCount++
	}

	if childrenCount == 0 {
		t.Errorf("GetChildrenHashesOfEvent failed, expected non-zero count, got %d", childrenCount)
	}
}

func Test_Index_GetDirectChildrenOfEvent(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents:     100,
		returnAllEvents: true,
	})

	var childrenCount int // todo this is not a good test

	for _, evHash := range evs {
		children, err := ou.Index.GetDirectChildrenOfEvent(evHash)
		if err != nil {
			t.Errorf("GetDirectChildrenOfEvent failed with error: %v", err)
		}
		childrenCount += len(children)

	}

	if childrenCount == 0 {
		t.Errorf("GetDirectChildrenOfEvent failed, expected non-zero count, got %d", childrenCount)
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
			EventToAppendTo: types.Event{EventIdentifier: types.EventIdentifier{EventHash: evHash}},
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
		ev       types.Event
		metadata []byte
		file     []byte
	}

	storageEvnets := make([]storageEvent, 0)

	for _, data := range testData {
		sev, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: types.Event{EventIdentifier: types.EventIdentifier{EventHash: rootEv}},
			Metadata:        data.metadata,
			File:            data.file,
		})
		if err != nil {
			t.Errorf("StoreFile failed with error: %v", err)
		}

		if sev.Content == nil || len(sev.Content) == 0 {
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
		ev       types.Event
		metadata []byte
		file     []byte
	}

	storageEvnets := make([]storageEvent, 0)

	for _, data := range testData {
		sev, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: types.Event{EventIdentifier: types.EventIdentifier{EventHash: rootEv}},
			Metadata:        data.metadata,
			File:            data.file,
		})
		if err != nil {
			t.Errorf("StoreFile failed with error: %v", err)
		}

		if sev.Content == nil || len(sev.Content) == 0 {
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
		ParentEvent: evs[0],
	})
	if err != nil {
		t.Errorf("CreateNewEvent failed with error: %v", err)
	}
}

func Test_DB_fastMeta(t *testing.T) {
	ou, evs := setupDBWithData(t, setupDBConfig{
		totalEvents: 1,
	})

	rootEv := evs[0]

	fm := types.FastMeta{
		types.FastMetaParameter([]byte("test1")),
		types.FastMetaParameter([]byte("test2")),
		types.FastMetaParameter([]byte("test3")),
		types.FastMetaParameter([]byte("test4")),
		types.FastMetaParameter([]byte("test5")),
		types.FastMetaParameter([]byte("test6")),
	}

	ev, err := ou.DB.CreateNewEvent(storage.EventOptions{
		ParentEvent: rootEv,
		FastMeta:    fm,
	})
	if err != nil {
		t.Errorf("CreateNewEvent failed with error: %v", err)
	}

	evGet, err := ou.DB.GetEvent(ev.EventIdentifier.EventHash)
	if err != nil {
		t.Errorf("GetEvent failed with error: %v", err)
	}

	for i, param := range evGet.FastMeta {
		if string(param) != string(fm[i]) {
			t.Errorf("FastMeta failed, expected %s, got %s", fm[i], param)
		}
	}
}

func Example() {
	// Initialize OuroborosDB with basic configuration
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{"ExamplePath/" + time.Now().String()}, // Directory for data storage
		MinimumFreeGB:             1,                                              // Minimum free space in GB
		GarbageCollectionInterval: 10,                                             // GC interval in seconds
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Failed to initialize OuroborosDB: %s", err))
	}
	defer ou.Close()

	// Ensure index is rebuilt for data integrity
	if _, err := ou.Index.RebuildIndex(); err != nil {
		log.Fatal(fmt.Sprintf("Error rebuilding index: %s", err))
	}

	uniqueNameBypass := time.Now().String()

	// Create a root event titled "ExampleRoot"
	rootEvent, err := ou.DB.CreateRootEvent("ExampleRoot" + uniqueNameBypass)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating root event: %s", err))
	}
	fmt.Printf("Created root event")

	// Create a child event under the "ExampleRoot" event
	_, err = ou.DB.CreateNewEvent(storage.EventOptions{
		ParentEvent: rootEvent.EventHash,
	})
	if err != nil {
		log.Fatal(fmt.Sprintf("Error creating child event: %s", err))
	}
	fmt.Printf("Created child event\n")

	// Rebuild index after creating the events
	if _, err := ou.Index.RebuildIndex(); err != nil {
		log.Fatal(fmt.Sprintf("Error rebuilding index: %s", err))
	}

	// Retrieve root events with the title "ExampleRoot"
	rootEvents, err := ou.DB.GetRootEventsWithTitle("ExampleRoot" + uniqueNameBypass)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error retrieving root events: %s", err))
	}
	for range rootEvents {
		fmt.Printf("Retrieved root event\n")
	}

	// Retrieve direct children of the "ExampleRoot" event
	children, err := ou.Index.GetDirectChildrenOfEvent(rootEvent.EventHash)
	if err != nil {
		log.Fatal(fmt.Sprintf("Error retrieving children of root event: %s", err))
	}
	for range children {
		fmt.Printf("Retrieved child event\n")
	}
}

func Benchmark_setupDBWithData(b *testing.B) {
	b.Run("RebuildIndex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			setupDBWithData(b, setupDBConfig{
				totalEvents: 10000,
			})
		}
	})

}

func Benchmark_Index_RebuildingIndex(b *testing.B) {
	ou, _ := setupDBWithData(b, setupDBConfig{
		totalEvents:   10000,
		notBuildIndex: true,
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

func Benchmark_DB_StoreFile(b *testing.B) {
	ou, evsHashes := setupDBWithData(b, setupDBConfig{
		totalEvents:        10000,
		returnRandomEvents: 1000,
	})

	ev := make([]types.Event, len(evsHashes))
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

func Benchmark_DB_GetFile(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
	})

	storeEvs := make([]types.Event, 0)

	for ev := range evs {
		storeEv, err := ou.DB.StoreFile(storage.StoreFileOptions{
			EventToAppendTo: types.Event{EventIdentifier: types.EventIdentifier{EventHash: evs[ev]}},
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

func Benchmark_DB_GetMetadata(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:          10000,
		returnRandomEvents:   1000,
		generateWithMetaData: true,
	})

	storeEvs := make([]types.Event, 0)

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

func Benchmark_DB_CreateNewEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents: 1000,
	})

	b.Run("CreateNewEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.CreateNewEvent(storage.EventOptions{
				ParentEvent: evs[rand.Intn(len(evs))],
			})
			if err != nil {
				b.Errorf("CreateNewEvent failed with error: %v", err)
			}
		}
	})
}

func Benchmark_DB_fastMeta(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents: 1,
	})

	rootEv := evs[0]

	fm := types.FastMeta{
		types.FastMetaParameter([]byte("test1")),
		types.FastMetaParameter([]byte("test2")),
		types.FastMetaParameter([]byte("test3")),
		types.FastMetaParameter([]byte("test4")),
		types.FastMetaParameter([]byte("test5")),
		types.FastMetaParameter([]byte("test6")),
	}

	b.Run("CreateNewEvent with FastMeta", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.DB.CreateNewEvent(storage.EventOptions{
				ParentEvent: rootEv,
				FastMeta:    fm,
			})
			if err != nil {
				b.Errorf("CreateNewEvent failed with error: %v", err)
			}
		}
	})

	ev, err := ou.DB.CreateNewEvent(storage.EventOptions{
		ParentEvent: rootEv,
		FastMeta:    fm,
	})
	if err != nil {
		b.Errorf("CreateNewEvent failed with error: %v", err)
	}

	b.Run("GetEvent with FastMeta", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			evGet, err := ou.DB.GetEvent(ev.EventIdentifier.EventHash)
			if err != nil {
				b.Errorf("GetEvent failed with error: %v", err)
			}
			for i, param := range evGet.FastMeta {
				if string(param) != string(fm[i]) {
					b.Errorf("FastMeta failed, expected %s, got %s", fm[i], param)
				}
			}
		}
	})
}

func Benchmark_Index_GetParentHashOfEvent(b *testing.B) {
	ou, evs := setupDBWithData(b, setupDBConfig{
		totalEvents:     1000,
		returnAllEvents: true,
		eventLevels:     5,
	})

	b.Run("GetParentHashOfEvent", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			evHash := evs[rand.Intn(len(evs))]
			_ = ou.Index.GetParentHashOfEvent(evHash)
		}
	})
}
