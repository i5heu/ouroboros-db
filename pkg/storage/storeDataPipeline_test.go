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
