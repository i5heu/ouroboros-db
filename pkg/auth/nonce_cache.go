package auth

import (
	"sync"
	"time"
)

// NonceCache tracks recently seen handshake nonces to
// prevent replay attacks. Expired entries are evicted
// inline during RecordNonce.
type NonceCache struct { // A
	mu      sync.Mutex
	entries map[[32]byte]time.Time
	ttl     time.Duration
	clock   Clock
	maxEntries int
}

// NewNonceCache creates a NonceCache with the given TTL
// and clock.
func NewNonceCache( // A
	ttl time.Duration,
	clock Clock,
) *NonceCache {
	return NewNonceCacheWithMaxEntries(
		ttl,
		clock,
		10_000,
	)
}

// NewNonceCacheWithMaxEntries creates a NonceCache
// with a hard cap on retained nonces.
func NewNonceCacheWithMaxEntries( // A
	ttl time.Duration,
	clock Clock,
	maxEntries int,
) *NonceCache {
	if maxEntries <= 0 {
		maxEntries = 10_000
	}
	return &NonceCache{
		entries: make(map[[32]byte]time.Time),
		ttl:     ttl,
		clock:   clock,
		maxEntries: maxEntries,
	}
}

// RecordNonce records a nonce and returns true if it was
// fresh (not previously seen). Returns false if the
// nonce is a replay or is zero.
func (nc *NonceCache) RecordNonce( // A
	nonce [32]byte,
) bool {
	if nonce == [32]byte{} {
		return false
	}

	nc.mu.Lock()
	defer nc.mu.Unlock()

	nc.cleanup()

	if _, exists := nc.entries[nonce]; exists {
		return false
	}

	nc.evictOldestIfFull()

	nc.entries[nonce] = nc.clock.Now()
	return true
}

// cleanup evicts expired entries. Must be called with
// mu held.
func (nc *NonceCache) cleanup() { // A
	cutoff := nc.clock.Now().Add(-nc.ttl)
	for k, v := range nc.entries {
		if v.Before(cutoff) {
			delete(nc.entries, k)
		}
	}
}

// evictOldestIfFull removes the oldest nonce entry if
// the cache reached its configured capacity. Must be
// called with mu held.
func (nc *NonceCache) evictOldestIfFull() { // A
	if len(nc.entries) < nc.maxEntries {
		return
	}

	var oldestNonce [32]byte
	var oldestTime time.Time
	found := false
	for nonce, seenAt := range nc.entries {
		if !found || seenAt.Before(oldestTime) {
			oldestNonce = nonce
			oldestTime = seenAt
			found = true
		}
	}
	if found {
		delete(nc.entries, oldestNonce)
	}
}
