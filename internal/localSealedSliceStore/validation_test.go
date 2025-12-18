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
		delete(set, sh)
		if len(set) == 0 {
			delete(m.byChunk, chk)
		}
	}
	for chk, byPk := range m.byChunkPub {
		for pk, set := range byPk {
			delete(set, sh)
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
	rsData := rapid.Uint8Range(1, 128).Draw(t, "rsData")
	rsParity := rapid.Uint8Range(1, 128).Draw(t, "rsParity")
	// Compute sum in a wider type and clamp to 255 before converting to uint8 to
	// avoid overflow.
	sum := uint16(rsData) + uint16(rsParity)
	var maxIndex uint8
	if sum == 0 {
		maxIndex = 0
	} else {
		s := sum - 1
		// Only convert to uint8 after ensuring s fits in the uint8 range.
		if s > 255 {
			maxIndex = 255
		} else {
			maxIndex = uint8(s)
		}
	}
	idx := rapid.Uint8Range(0, maxIndex).Draw(t, "idx")
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
		defer func() {
			if err := db.Close(); err != nil {
				t.Fatalf("close badger: %v", err)
			}
		}()

		st := newPBTState(local.New(db))

		steps := rapid.IntRange(50, 200).Draw(t, "steps")
		for i := 0; i < steps; i++ {
			op := rapid.IntRange(0, 3).Draw(t, "op")

			switch op {
			case 0: // Store
				st.opStore(t)

			case 1: // Get
				st.opGet(t)

			case 2: // Delete
				st.opDelete(t)

			case 3: // List checks (global invariant sampling)
				st.opListChecks(t)
			}

			// Optional: after every step, you can sample-check some known hashes.
			// (Often catches indexing bugs faster.)
		}
	})
}

type pbtState struct {
	s     local.LocalSealedSliceStore
	m     *model
	known []hash.Hash
}

func newPBTState(s local.LocalSealedSliceStore) *pbtState { // A
	return &pbtState{
		s: s,
		m: newModel(),
	}
}

func (st *pbtState) opStore(t *rapid.T) { // A
	chunk := genHash(t)
	pub := genHash(t)
	ss := genSealedSlice(t, chunk)

	gotHash, err := st.s.Store(ss, pub)
	if err != nil {
		t.Fatalf("Store error: %v", err)
	}

	enc := mustEncodeSealedSlice(t, ss)
	st.m.applyStore(chunk, pub, gotHash, enc)
	st.known = append(st.known, gotHash)
}

func (st *pbtState) opGet(t *rapid.T) { // A
	if len(st.known) == 0 {
		return
	}

	h := st.known[pickIndex(t, len(st.known), "pickGet")]
	ss, err := st.s.Get(h)

	encExpected, ok := st.m.bySlice[key(h)]
	if !ok {
		if err == nil {
			t.Fatalf("expected Get to error for missing hash")
		}
		return
	}
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}

	encGot := mustEncodeSealedSlice(t, ss)
	if !bytes.Equal(encGot, encExpected) {
		t.Fatalf("Get mismatch for hash %x", h[:])
	}
}

func (st *pbtState) opDelete(t *rapid.T) { // A
	if len(st.known) == 0 {
		return
	}

	h := st.known[pickIndex(t, len(st.known), "pickDel")]
	_ = st.s.Delete(h) // choose semantics: deleting missing can be nil or error
	st.m.applyDelete(h)
}

func (st *pbtState) opListChecks(t *rapid.T) { // A
	// Pick a random chunk/pubkey and compare lists to model.
	chunk := genHash(t)
	pub := genHash(t)

	got1, err := st.s.ChunkListSealedSlices(chunk)
	if err != nil {
		t.Fatalf("ChunkListSealedSlices error: %v", err)
	}
	wantSet := st.m.byChunk[key(chunk)]
	want1 := setToHashes(wantSet)
	sortHashes(got1)
	sortHashes(want1)
	if !equalHashes(got1, want1) {
		t.Fatalf("ChunkListSealedSlices mismatch")
	}

	got2, err := st.s.ChunkListSealedSlicesForPubKey(chunk, pub)
	if err != nil {
		t.Fatalf("ChunkListSealedSlicesForPubKey error: %v", err)
	}
	wantSet2 := st.m.byChunkPub[key(chunk)][key(pub)]
	want2 := setToHashes(wantSet2)
	sortHashes(got2)
	sortHashes(want2)
	if !equalHashes(got2, want2) {
		t.Fatalf("ChunkListSealedSlicesForPubKey mismatch")
	}
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
