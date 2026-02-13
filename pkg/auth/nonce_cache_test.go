package auth

import (
	"sync"
	"testing"
	"time"
)

func TestNonceCacheRecordNonceFreshThenReplay( // A
	t *testing.T,
) {
	t.Parallel()
	clk := &fakeClock{now: time.Now().UTC()}
	nc := NewNonceCache(5*time.Minute, clk)
	nonce := testNonce(t)

	if !nc.RecordNonce(nonce) {
		t.Fatal("first nonce record should be fresh")
	}
	if nc.RecordNonce(nonce) {
		t.Fatal("second nonce record should be replay")
	}
}

func TestNonceCacheRejectsZeroNonce( // A
	t *testing.T,
) {
	t.Parallel()
	clk := &fakeClock{now: time.Now().UTC()}
	nc := NewNonceCache(5*time.Minute, clk)

	if nc.RecordNonce([32]byte{}) {
		t.Fatal("zero nonce must be rejected")
	}
}

func TestNonceCacheTTLEvictionAllowsReuse( // A
	t *testing.T,
) {
	t.Parallel()
	base := time.Now().UTC()
	clk := &fakeClock{now: base}
	nc := NewNonceCache(time.Minute, clk)
	nonce := testNonce(t)

	if !nc.RecordNonce(nonce) {
		t.Fatal("first nonce record should be fresh")
	}

	clk.now = base.Add(2 * time.Minute)
	if !nc.RecordNonce(nonce) {
		t.Fatal("expired nonce should be reusable")
	}
}

func TestNonceCacheCleanupEvictsExpiredOnly( // A
	t *testing.T,
) {
	t.Parallel()
	base := time.Now().UTC()
	clk := &fakeClock{now: base}
	nc := NewNonceCache(time.Minute, clk)

	oldNonce := testNonce(t)
	freshNonce := testNonce(t)

	nc.mu.Lock()
	nc.entries[oldNonce] = base.Add(-2 * time.Minute)
	nc.entries[freshNonce] = base.Add(-30 * time.Second)
	nc.cleanup()
	nc.mu.Unlock()

	nc.mu.Lock()
	_, oldExists := nc.entries[oldNonce]
	_, freshExists := nc.entries[freshNonce]
	nc.mu.Unlock()

	if oldExists {
		t.Fatal("expired nonce should be evicted")
	}
	if !freshExists {
		t.Fatal("fresh nonce should remain")
	}
}

func TestNonceCacheConcurrentRecordNonce( // A
	t *testing.T,
) {
	t.Parallel()
	clk := &fakeClock{now: time.Now().UTC()}
	nc := NewNonceCache(5*time.Minute, clk)
	nonce := testNonce(t)

	var wg sync.WaitGroup
	results := make(chan bool, 64)
	for range 64 {
		wg.Add(1)
		go func() { // A
			defer wg.Done()
			results <- nc.RecordNonce(nonce)
		}()
	}
	wg.Wait()
	close(results)

	freshCount := 0
	for r := range results {
		if r {
			freshCount++
		}
	}
	if freshCount != 1 {
		t.Fatalf("freshCount = %d, want 1", freshCount)
	}
}
