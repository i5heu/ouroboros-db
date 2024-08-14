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

func BenchmarkStoreDataPipelineChaCha20(b *testing.B) {
	// Set up data and storage instance before the benchmark loop
	testData := []byte("hello world ")
	data := make([]byte, 0)

	for i := 0; i < 10000; i++ {
		data = append(data, testData...)
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

func BenchmarkStoreDataPipelineAES(b *testing.B) {
	// Set up data and storage instance before the benchmark loop
	testData := []byte("hello world ")
	data := make([]byte, 0)

	for i := 0; i < 10000; i++ {
		data = append(data, testData...)
	}

	storage := Storage{
		wp: workerPool.NewWorkerPool(workerPool.Config{GlobalBuffer: 1000}),
	}

	// Reset timer to exclude setup time from the performance measurement
	b.ResetTimer()

	// Perform the operation b.N times
	for i := 0; i < b.N; i++ {
		_, _, err := storage.StoreDataPipelineAES(data)
		if err != nil {
			b.Errorf("Error: %v", err)
		}
	}
}
