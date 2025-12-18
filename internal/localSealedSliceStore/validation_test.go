package localSealedSliceStore

import (
	"bytes"
	"context"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cas"
	"pgregory.net/rapid"
)

func TestLocalStorePropertyLifecycle(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		db := openInMemoryDB(t)
		defer func() {
			_ = db.Close()
		}()

		store := New(db)
		chunkHash := randomHash(t)

		pubKeys := randomPubKeys(t)
		sliceCount := rapid.IntRange(1, 5).Draw(t, "sliceCount")

		var stored []cas.SealedSliceWithPayload
		var storedPubKeys []hash.Hash

		for i := 0; i < sliceCount; i++ {
			slice := randomSealedSlice(t, chunkHash)
			pubKey := pubKeys[rapid.IntRange(0, len(pubKeys)-1).Draw(t, "pubKeyIndex")]

			returnedHash, err := store.Store(slice, pubKey)
			if err != nil {
				t.Fatalf("store returned error: %v", err)
			}
			if returnedHash != slice.Hash {
				t.Fatalf(
					"store returned unexpected hash: got %s want %s",
					returnedHash,
					slice.Hash,
				)
			}

			stored = append(stored, slice)
			storedPubKeys = append(storedPubKeys, pubKey)
		}

		chunkHashes, err := store.ChunkListSealedSlices(chunkHash)
		if err != nil {
			t.Fatalf("chunk list failed: %v", err)
		}
		if !unorderedHashEqual(chunkHashes, collectHashes(stored)) {
			t.Fatalf(
				"chunk list mismatch: got %d hashes expected %d",
				len(chunkHashes),
				len(stored),
			)
		}

		for _, pubKey := range uniqueHashes(storedPubKeys) {
			pubHashes, err := store.ChunkListSealedSlicesForPubKey(chunkHash, pubKey)
			if err != nil {
				t.Fatalf("chunk list for pubkey failed: %v", err)
			}
			if !unorderedHashEqual(
				pubHashes,
				collectPubHashes(stored, storedPubKeys, pubKey),
			) {
				t.Fatalf("chunk list for pubkey mismatch for %s", pubKey)
			}
		}

		for i, slice := range stored {
			got, err := store.Get(slice.Hash)
			if err != nil {
				t.Fatalf("get failed for %s: %v", slice.Hash, err)
			}
			if got.Hash != slice.Hash ||
				got.ChunkHash != slice.ChunkHash ||
				got.RSDataSlices != slice.RSDataSlices ||
				got.RSParitySlices != slice.RSParitySlices ||
				got.RSSliceIndex != slice.RSSliceIndex ||
				!bytes.Equal(got.Nonce, slice.Nonce) {
				t.Fatalf("retrieved slice metadata mismatch at index %d", i)
			}

			payload, err := got.GetSealedPayload(context.Background())
			if err != nil {
				t.Fatalf("get payload failed: %v", err)
			}
			if !bytes.Equal(payload, slice.SealedPayload) {
				t.Fatalf("payload mismatch at index %d", i)
			}
		}

		deleteIndex := rapid.IntRange(0, len(stored)-1).Draw(t, "deleteIndex")
		deleteHash := stored[deleteIndex].Hash

		if err := store.Delete(deleteHash); err != nil {
			t.Fatalf("delete failed: %v", err)
		}
		if _, err := store.Get(deleteHash); err == nil {
			t.Fatalf("get succeeded after delete for %s", deleteHash)
		}

		expectedRemaining := removeHash(collectHashes(stored), deleteHash)
		chunkHashesAfterDelete, err := store.ChunkListSealedSlices(chunkHash)
		if err != nil {
			t.Fatalf("chunk list after delete failed: %v", err)
		}
		if !unorderedHashEqual(chunkHashesAfterDelete, expectedRemaining) {
			t.Fatalf("chunk list after delete mismatch")
		}
	})
}

func openInMemoryDB(t *rapid.T) *badger.DB {
	db, err := badger.Open(
		badger.DefaultOptions("").WithInMemory(true),
	)
	if err != nil {
		t.Fatalf("failed to open badger: %v", err)
	}
	return db
}

func randomHash(t *rapid.T) hash.Hash {
	bytes := rapid.SliceOfN(
		rapid.Byte(),
		hashLength,
		hashLength,
	).Draw(
		t,
		"hashBytes",
	)
	var h hash.Hash
	copy(h[:], bytes)
	if h == (hash.Hash{}) {
		h[0] = 1 // avoid zero hash
	}
	return h
}

func randomPubKeys(t *rapid.T) []hash.Hash {
	count := rapid.IntRange(1, 3).Draw(t, "pubKeyCount")
	keys := make([]hash.Hash, count)
	for i := 0; i < count; i++ {
		keys[i] = randomHash(t)
	}
	return keys
}

func randomBytes(t *rapid.T, min, max int) []byte {
	size := rapid.IntRange(min, max).Draw(t, "byteCount")
	return rapid.SliceOfN(rapid.Byte(), size, size).Draw(t, "bytes")
}

func randomSealedSlice(
	t *rapid.T,
	chunkHash hash.Hash,
) cas.SealedSliceWithPayload {
	rsData := rapid.Uint8Range(1, 6).Draw(t, "rsData")
	rsParity := rapid.Uint8Range(1, 6).Draw(t, "rsParity")
	total := int(rsData) + int(rsParity)
	rsIndex := uint8(rapid.IntRange(0, total-1).Draw(t, "rsIndex"))

	nonce := randomBytes(t, 12, 32)
	payload := randomBytes(t, 1, 512)

	computedHash := computeSliceHash(
		chunkHash,
		rsData,
		rsParity,
		rsIndex,
		nonce,
		payload,
	)

	return cas.SealedSliceWithPayload{
		SealedSlice: cas.SealedSlice{
			Hash:           computedHash,
			ChunkHash:      chunkHash,
			RSDataSlices:   rsData,
			RSParitySlices: rsParity,
			RSSliceIndex:   rsIndex,
			Nonce:          nonce,
		},
		SealedPayload: payload,
	}
}

func collectHashes(slices []cas.SealedSliceWithPayload) []hash.Hash {
	hashes := make([]hash.Hash, 0, len(slices))
	for _, s := range slices {
		hashes = append(hashes, s.Hash)
	}
	return hashes
}

func collectPubHashes(
	slices []cas.SealedSliceWithPayload,
	pubKeys []hash.Hash,
	target hash.Hash,
) []hash.Hash {
	var hashes []hash.Hash
	for i, s := range slices {
		if pubKeys[i] == target {
			hashes = append(hashes, s.Hash)
		}
	}
	return hashes
}

func unorderedHashEqual(a, b []hash.Hash) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[hash.Hash]int, len(a))
	for _, h := range a {
		counts[h]++
	}
	for _, h := range b {
		counts[h]--
		if counts[h] < 0 {
			return false
		}
	}
	return true
}

func uniqueHashes(in []hash.Hash) []hash.Hash {
	seen := make(map[hash.Hash]struct{}, len(in))
	var out []hash.Hash
	for _, h := range in {
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	return out
}

func removeHash(in []hash.Hash, target hash.Hash) []hash.Hash {
	var out []hash.Hash
	for _, h := range in {
		if h != target {
			out = append(out, h)
		}
	}
	return out
}
