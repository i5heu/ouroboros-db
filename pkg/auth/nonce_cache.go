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
}

// NewNonceCache creates a NonceCache with the given TTL
// and clock.
func NewNonceCache( // A
	ttl time.Duration,
	clock Clock,
) *NonceCache {
	return &NonceCache{
		entries: make(map[[32]byte]time.Time),
		ttl:     ttl,
		clock:   clock,
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
