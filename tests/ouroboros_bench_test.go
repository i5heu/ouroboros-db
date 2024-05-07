package ouroboros_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db/internal/storage"
)

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
				HashOfParentEvent: evs[rand.Intn(len(evs))],
			})
			if err != nil {
				b.Errorf("CreateNewEvent failed with error: %v", err)
			}
		}
	})
}
