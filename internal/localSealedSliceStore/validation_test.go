package localSealedSliceStore_test

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	local "github.com/i5heu/ouroboros-db/internal/localSealedSliceStore"
	"github.com/i5heu/ouroboros-db/pkg/cas"
	"pgregory.net/rapid"
)

type model struct {
	// canonical bytes keyed by slice hash
	bySlice map[string][]byte

	// chunk -> set(sliceHash)
	byChunk map[string]map[string]struct{}

	// chunk -> pubkey -> set(sliceHash)
	byChunkPub map[string]map[string]map[string]struct{}
}

func newModel() *model {
	return &model{
		bySlice:    map[string][]byte{},
		byChunk:    map[string]map[string]struct{}{},
		byChunkPub: map[string]map[string]map[string]struct{}{},
	}
}

func key(
	h hash.Hash,
) string {
	return string(h[:])
} // adjust if hash.Hash is not [N]byte

func ensureSet(m map[string]struct{}) map[string]struct{} {
	if m == nil {
		return map[string]struct{}{}
	}
	return m
}

func (m *model) applyStore(chunk, pub, sliceHash hash.Hash, enc []byte) {
	chk := key(chunk)
	pk := key(pub)
	sh := key(sliceHash)

	m.bySlice[sh] = append([]byte(nil), enc...)

	m.byChunk[chk] = ensureSet(m.byChunk[chk])
	m.byChunk[chk][sh] = struct{}{}

	if m.byChunkPub[chk] == nil {
		m.byChunkPub[chk] = map[string]map[string]struct{}{}
	}
	m.byChunkPub[chk][pk] = ensureSet(m.byChunkPub[chk][pk])
	m.byChunkPub[chk][pk][sh] = struct{}{}
}

func (m *model) applyDelete(sliceHash hash.Hash) {
	sh := key(sliceHash)
	delete(m.bySlice, sh)

	// Remove from all indexes
	for chk, set := range m.byChunk {
		if _, ok := set[sh]; ok {
			delete(set, sh)
		}
		if len(set) == 0 {
			delete(m.byChunk, chk)
		}
	}
	for chk, byPk := range m.byChunkPub {
		for pk, set := range byPk {
			if _, ok := set[sh]; ok {
				delete(set, sh)
			}
			if len(set) == 0 {
				delete(byPk, pk)
			}
		}
		if len(byPk) == 0 {
			delete(m.byChunkPub, chk)
		}
	}
}

func setToHashes(set map[string]struct{}) []hash.Hash {
	out := make([]hash.Hash, 0, len(set))
	for sh := range set {
		var h hash.Hash
		copy(h[:], []byte(sh))
		out = append(out, h)
	}
	return out
}

func sortHashes(xs []hash.Hash) {
	sort.Slice(xs, func(i, j int) bool {
		return bytes.Compare(xs[i][:], xs[j][:]) < 0
	})
}

func mustEncodeSealedSlice(t *rapid.T, ss any) []byte {
	t.Helper()
	var chunkHash hash.Hash
	var rsData, rsParity, rsIndex uint8
	var nonce, payload []byte

	switch v := ss.(type) {
	case cas.SealedSlice:
		chunkHash = v.ChunkHash
		rsData = v.RSDataSlices
		rsParity = v.RSParitySlices
		rsIndex = v.RSSliceIndex
		nonce = v.Nonce
		var err error
		payload, err = v.GetSealedPayload(context.Background())
		if err != nil {
			t.Fatalf("failed to get payload: %v", err)
		}
	case cas.SealedSliceWithPayload:
		chunkHash = v.ChunkHash
		rsData = v.RSDataSlices
		rsParity = v.RSParitySlices
		rsIndex = v.RSSliceIndex
		nonce = v.Nonce
		payload = v.SealedPayload
	default:
		t.Fatalf("unexpected type %T", ss)
	}

	buf := make([]byte, 0, len(chunkHash)+3+len(nonce)+len(payload))
	buf = append(buf, chunkHash[:]...)
	buf = append(buf, rsData, rsParity, rsIndex)
	buf = append(buf, nonce...)
	buf = append(buf, payload...)
	return buf
}

func genHash(t *rapid.T) hash.Hash {
	var h hash.Hash
	b := rapid.SliceOfN(rapid.Byte(), len(h), len(h)).Draw(t, "hashBytes")
	copy(h[:], b)
	return h
}

func genSealedSlice(t *rapid.T, chunk hash.Hash) cas.SealedSliceWithPayload {
	rsData := uint8(rapid.IntRange(1, 128).Draw(t, "rsData"))
	rsParity := uint8(rapid.IntRange(1, 128).Draw(t, "rsParity"))
	idx := uint8(rapid.IntRange(0, int(rsData)+int(rsParity)-1).Draw(t, "idx"))
	nonce := rapid.SliceOfN(rapid.Byte(), 12, 12).Draw(t, "nonce")
	payload := rapid.SliceOfN(rapid.Byte(), 1, 1024).Draw(t, "payload")

	ss := cas.SealedSliceWithPayload{
		SealedSlice: cas.SealedSlice{
			ChunkHash:      chunk,
			RSDataSlices:   rsData,
			RSParitySlices: rsParity,
			RSSliceIndex:   idx,
			Nonce:          nonce,
		},
		SealedPayload: payload,
	}

	// Compute hash
	buf := make([]byte, 0, len(chunk)+3+len(nonce)+len(payload))
	buf = append(buf, chunk[:]...)
	buf = append(buf, rsData, rsParity, idx)
	buf = append(buf, nonce...)
	buf = append(buf, payload...)
	ss.Hash = hash.HashBytes(buf)

	return ss
}

func Test_LocalSealedSliceStore_StatefulPBT(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		opt := badger.DefaultOptions("").WithInMemory(true)
		opt = opt.WithLogger(nil)

		db, err := badger.Open(opt)
		if err != nil {
			t.Fatalf("open badger: %v", err)
		}
		defer db.Close()

		s := local.New(db)
		m := newModel()

		// Keep some known hashes around so Get/Delete are meaningful.
		var known []hash.Hash

		steps := rapid.IntRange(50, 200).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			op := rapid.IntRange(0, 3).Draw(t, "op")

			switch op {
			case 0: // Store
				chunk := genHash(t)
				pub := genHash(t)
				ss := genSealedSlice(t, chunk)

				gotHash, err := s.Store(ss, pub)
				if err != nil {
					t.Fatalf("Store error: %v", err)
				}

				enc := mustEncodeSealedSlice(t, ss)
				m.applyStore(chunk, pub, gotHash, enc)
				known = append(known, gotHash)

			case 1: // Get
				if len(known) == 0 {
					continue
				}
				h := known[pickIndex(t, len(known), "pickGet")]
				ss, err := s.Get(h)

				encExpected, ok := m.bySlice[key(h)]
				if !ok {
					// should be deleted / missing
					if err == nil {
						t.Fatalf("expected Get to error for missing hash")
					}
					continue
				}
				if err != nil {
					t.Fatalf("Get error: %v", err)
				}
				encGot := mustEncodeSealedSlice(t, ss)
				if !bytes.Equal(encGot, encExpected) {
					t.Fatalf("Get mismatch for hash %x", h[:])
				}

			case 2: // Delete
				if len(known) == 0 {
					continue
				}
				h := known[pickIndex(t, len(known), "pickDel")]
				_ = s.Delete(
					h,
				) // choose semantics: deleting missing can be nil or error
				m.applyDelete(h)

			case 3: // List checks (global invariant sampling)
				// Pick a random chunk/pubkey and compare lists to model.
				chunk := genHash(t)
				pub := genHash(t)

				got1, err := s.ChunkListSealedSlices(chunk)
				if err != nil {
					t.Fatalf("ChunkListSealedSlices error: %v", err)
				}
				wantSet := m.byChunk[key(chunk)]
				want1 := setToHashes(wantSet)
				sortHashes(got1)
				sortHashes(want1)
				if !equalHashes(got1, want1) {
					t.Fatalf("ChunkListSealedSlices mismatch")
				}

				got2, err := s.ChunkListSealedSlicesForPubKey(chunk, pub)
				if err != nil {
					t.Fatalf("ChunkListSealedSlicesForPubKey error: %v", err)
				}
				wantSet2 := m.byChunkPub[key(chunk)][key(pub)]
				want2 := setToHashes(wantSet2)
				sortHashes(got2)
				sortHashes(want2)
				if !equalHashes(got2, want2) {
					t.Fatalf("ChunkListSealedSlicesForPubKey mismatch")
				}
			}

			// Optional: after every step, you can sample-check some known hashes.
			// (Often catches indexing bugs faster.)
		}
	})
}

func equalHashes(a, b []hash.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i][:], b[i][:]) {
			return false
		}
	}
	return true
}

func pickIndex(t *rapid.T, n int, label string) int {
	if n <= 0 {
		t.Fatalf("pickIndex called with n=%d", n)
	}
	return rapid.IntRange(0, n-1).Draw(t, label)
}
