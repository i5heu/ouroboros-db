package ouroboros_test

import (
	"bytes"
	"fmt"
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

func setupDBWithData(t testing.TB, conf setupDBConfig) (*ouroboros.OuroborosDB, []types.Event) {
	if conf.totalEvents == 0 {
		conf.totalEvents = 1000
	}
	if conf.eventLevels < 1 {
		conf.eventLevels = 3
	}

	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir(), t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOuroborosDB failed with error: %v", err)
	}

	allEvents := make([]types.Event, 0, conf.totalEvents)

	// Create the root event
	rootEv, err := ou.DB.CreateRootEvent("RootEvent_Level_0")
	if err != nil {
		t.Fatalf("CreateRootEvent failed with error: %v", err)
	}
	allEvents = append(allEvents, rootEv)

	// Calculate number of events per level
	eventsPerLevel := (conf.totalEvents - 1) / (conf.eventLevels - 1)
	if (conf.totalEvents-1)%(conf.eventLevels-1) != 0 {
		eventsPerLevel++ // Adjust for rounding
	}

	for i := 1; i < conf.eventLevels; i++ {
		for j := 0; j < eventsPerLevel; j++ {
			if len(allEvents) >= conf.totalEvents {
				break
			}

			metadata := []byte{}
			file := []byte{}
			if conf.generateWithMetaData {
				metadata = []byte(randomString(256))
			}
			if conf.generateWithFiles {
				file = []byte(randomString(1024))
			}

			parentEvent := allEvents[rand.Intn(len(allEvents))]
			event, err := ou.DB.StoreFile(storage.StoreFileOptions{
				EventToAppendTo: parentEvent,
				Metadata:        metadata,
				File:            file,
				Temporary:       types.Binary(false),
				FullTextSearch:  types.Binary(false),
			})
			if err != nil {
				t.Fatalf("StoreFile failed with error: %v", err)
			}
			allEvents = append(allEvents, event)
		}
	}

	if len(allEvents) != conf.totalEvents {
		t.Fatalf("expected %d events, got %d", conf.totalEvents, len(allEvents))
	}

	return ou, allEvents
}

func TestSetupDBWithData(t *testing.T) {
	conf := setupDBConfig{
		totalEvents:          10,
		eventLevels:          2,
		generateWithFiles:    true,
		generateWithMetaData: true,
	}

	ou, events := setupDBWithData(t, conf)
	defer ou.Close()

	if len(events) != conf.totalEvents {
		t.Fatalf("expected %d events, got %d", conf.totalEvents, len(events))
	}

	fmt.Println("TestSetupDBWithData successful")
}

func TestNewOuroborosDB(t *testing.T) {
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOuroborosDB failed with error: %v", err)
	}
	defer ou.Close()

	rootEvent, err := ou.DB.CreateRootEvent("TestRootEvent")
	if err != nil {
		t.Fatalf("CreateRootEvent failed with error: %v", err)
	}

	if rootEvent.EventType != types.Root {
		t.Fatalf("expected event type to be %d, got %d", types.Root, rootEvent.EventType)
	}

	fmt.Println("TestNewOuroborosDB successful")
}

func TestStoreFile(t *testing.T) {
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOuroborosDB failed with error: %v", err)
	}
	defer ou.Close()

	rootEvent, err := ou.DB.CreateRootEvent("TestRootEvent")
	if err != nil {
		t.Fatalf("CreateRootEvent failed with error: %v", err)
	}

	fileContent := []byte(randomString(1024))
	metadata := []byte(randomString(256))

	event, err := ou.DB.StoreFile(storage.StoreFileOptions{
		EventToAppendTo: rootEvent,
		Metadata:        metadata,
		File:            fileContent,
		Temporary:       types.Binary(false),
		FullTextSearch:  types.Binary(false),
	})
	if err != nil {
		t.Fatalf("StoreFile failed with error: %v", err)
	}

	if event.EventIdentifier.EventType != types.Normal {
		t.Fatalf("expected event type to be %d, got %d", types.Normal, event.EventIdentifier.EventType)
	}

	fmt.Println("TestStoreFile successful")
}

func TestGetFile(t *testing.T) {
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOuroborosDB failed with error: %v", err)
	}
	defer ou.Close()

	rootEvent, err := ou.DB.CreateRootEvent("TestRootEvent")
	if err != nil {
		t.Fatalf("CreateRootEvent failed with error: %v", err)
	}

	fileContent := []byte(randomString(1024))
	metadata := []byte(randomString(256))

	event, err := ou.DB.StoreFile(storage.StoreFileOptions{
		EventToAppendTo: rootEvent,
		Metadata:        metadata,
		File:            fileContent,
		Temporary:       types.Binary(false),
		FullTextSearch:  types.Binary(false),
	})
	if err != nil {
		t.Fatalf("StoreFile failed with error: %v", err)
	}

	retrievedFile, err := ou.DB.GetFile(event)
	if err != nil {
		t.Fatalf("GetFile failed with error: %v", err)
	}

	if string(retrievedFile) != string(fileContent) {
		t.Fatalf("expected file content to be %s, got %s", fileContent, retrievedFile)
	}

	fmt.Println("TestGetFile successful")
}

func TestStoreAndRetrieveEventWithFastMeta(t *testing.T) {
	ou, err := ouroboros.NewOuroborosDB(ouroboros.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10 * time.Minute,
	})
	if err != nil {
		t.Fatalf("NewOuroborosDB failed with error: %v", err)
	}
	defer ou.Close()

	rootEvent, err := ou.DB.CreateRootEvent("TestRootEventWithFastMeta")
	if err != nil {
		t.Fatalf("CreateRootEvent failed with error: %v", err)
	}

	fileContent := []byte{0xDE, 0xAD, 0xBE, 0xEF} // some real binary data
	metadata := []byte("Example metadata for testing fastMeta content")
	fastMeta := types.FastMeta{
		types.FastMetaParameter([]byte("key1")),
		types.FastMetaParameter([]byte("key2")),
		types.FastMetaParameter([]byte("key3")),
	}

	event, err := ou.DB.StoreFile(storage.StoreFileOptions{
		EventToAppendTo: rootEvent,
		FastMeta:        fastMeta,
		Metadata:        metadata,
		File:            fileContent,
		Temporary:       types.Binary(false),
		FullTextSearch:  types.Binary(false),
	})
	if err != nil {
		t.Fatalf("StoreFile with fastMeta failed with error: %v", err)
	}

	retrievedFile, err := ou.DB.GetFile(event)
	if err != nil {
		t.Fatalf("GetFile failed with error: %v", err)
	}

	if !bytes.Equal(retrievedFile, fileContent) {
		t.Fatalf("expected file content to be %v, got %v", fileContent, retrievedFile)
	}

	retrievedEvent, err := ou.DB.GetEvent(event.EventIdentifier.EventHash)
	if err != nil {
		t.Fatalf("GetEvent failed with error: %v", err)
	}

	// if !bytes.Equal(retrievedEvent.Metadata, metadata) {
	// 	t.Fatalf("expected metadata to be %s, got %s", string(metadata), string(retrievedEvent.Metadata))
	// }

	if len(retrievedEvent.FastMeta) != len(fastMeta) {
		t.Fatalf("expected fastMeta length to be %d, got %d", len(fastMeta), len(retrievedEvent.FastMeta))
	}

	for i, param := range fastMeta {
		if !bytes.Equal(retrievedEvent.FastMeta[i], param) {
			t.Fatalf("expected fastMeta parameter %d to be %s, got %s", i, string(param), string(retrievedEvent.FastMeta[i]))
		}
	}

	fmt.Println("TestStoreAndRetrieveEventWithFastMeta successful")
}
