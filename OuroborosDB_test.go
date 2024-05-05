package OuroborosDB_test

import (
	"OuroborosDB"
	"OuroborosDB/internal/storage"
	"testing"
	"time"
)

func setupIndex(t testing.TB, rowsIn1k int) *OuroborosDB.OuroborosDB {
	ou, err := OuroborosDB.NewOuroborosDB(OuroborosDB.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10,
	})
	if err != nil {
		t.Errorf("NewOuroborosDB failed with error: %v", err)
	}

	parents := rowsIn1k / 500

	for i := 0; i < parents; i++ {
		rootEv, err := ou.DB.CreateRootEvent(time.Now().String())
		if err != nil {
			t.Errorf("CreateRootEvent failed with error: %v", err)
		}

		for j := 0; j < 499; j++ {
			_, err := ou.DB.CreateNewEvent(storage.EventOptions{
				HashOfParentEvent: rootEv.EventHash,
			})
			if err != nil {
				t.Errorf("CreateNewEvent failed with error: %v", err)
			}
		}
	}

	time.Sleep(1 * time.Second)

	return ou
}

func TestIndex_RebuildIndex(t *testing.T) {
	testEvs := 1000
	ou := setupIndex(t, testEvs)

	// Rebuild the index
	indexedRows, err := ou.Index.RebuildIndex()
	if err != nil {
		t.Errorf("RebuildIndex failed with error: %v", err)
	}

	if indexedRows != uint64(testEvs) {
		t.Errorf("RebuildIndex failed, expected %d, got %d", testEvs, indexedRows)
	}
}

func BenchmarkRebuildingIndex10kRows(b *testing.B) {
	ou := setupIndex(b, 10000)

	b.Run("RebuildIndex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, err := ou.Index.RebuildIndex()
			if err != nil {
				b.Errorf("RebuildIndex failed with error: %v", err)
			}
		}
	})

}
