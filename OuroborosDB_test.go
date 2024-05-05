package OuroborosDB_test

import (
	"OuroborosDB"
	"OuroborosDB/internal/storage"
	"testing"
)

func setupIndex(t testing.TB, rows int) *OuroborosDB.OuroborosDB {
	ou, err := OuroborosDB.NewOuroborosDB(OuroborosDB.Config{
		Paths:                     []string{t.TempDir()},
		MinimumFreeGB:             1,
		GarbageCollectionInterval: 10,
	})
	if err != nil {
		t.Errorf("NewOuroborosDB failed with error: %v", err)
	}

	rev, err := ou.DB.CreateRootEvent("root1")
	if err != nil {
		t.Errorf("CreateRootEvent failed with error: %v", err)
	}

	rev2, err := ou.DB.CreateRootEvent("root2")
	if err != nil {
		t.Errorf("CreateRootEvent failed with error: %v", err)
	}

	for i := 0; i < rows; i++ {
		_, err := ou.DB.CreateNewEvent(storage.EventOptions{
			HashOfParentEvent: rev.EventHash,
		})
		if err != nil {
			t.Errorf("CreateNewEvent failed with error: %v", err)
		}
	}

	for i := 0; i < rows; i++ {
		_, err := ou.DB.CreateNewEvent(storage.EventOptions{
			HashOfParentEvent: rev2.EventHash,
		})
		if err != nil {
			t.Errorf("CreateNewEvent failed with error: %v", err)
		}
	}

	return ou
}

func TestIndex_RebuildIndex(t *testing.T) {
	ou := setupIndex(t, 10)

	// Rebuild the index
	err := ou.Index.RebuildIndex()
	if err != nil {
		t.Errorf("RebuildIndex failed with error: %v", err)
	}
}

func BenchmarkRebuildingIndex10kRows(b *testing.B) {
	ou := setupIndex(b, 5000)

	b.Run("RebuildIndex", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			err := ou.Index.RebuildIndex()
			if err != nil {
				b.Errorf("RebuildIndex failed with error: %v", err)
			}
		}
	})

}
