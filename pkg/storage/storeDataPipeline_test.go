package storage

import (
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/workerPool"
)

func TestStoreDataPipeline(t *testing.T) {
	b := []byte("hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world hello world")

	s := Storage{
		wp: workerPool.NewWorkerPool(workerPool.Config{GlobalBuffer: 1000}),
	}

	_, _, err := s.StoreDataPipeline(b)
	if err != nil {
		t.Errorf("Error: %v", err)
	}

}

func BenchmarkStoreDataPipeline(b *testing.B) {
	// Set up data and storage instance before the benchmark loop
	data := []byte("hello world ")

	for i := 0; i < 100; i++ {
		data = append(data, data...)
	}

	storage := Storage{
		wp: workerPool.NewWorkerPool(workerPool.Config{GlobalBuffer: 1000}),
	}

	// Reset timer to exclude setup time from the performance measurement
	b.ResetTimer()

	// Perform the operation b.N times
	for i := 0; i < b.N; i++ {
		_, _, err := storage.StoreDataPipeline(data)
		if err != nil {
			b.Errorf("Error: %v", err)
		}
	}
}
