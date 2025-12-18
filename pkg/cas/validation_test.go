package cas

import (
	"bytes"
	"context"
	"sort"
	"testing"
	"time"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"pgregory.net/rapid"
)

// --------- small generators / helpers ---------

func drawHash(t *rapid.T, label string) hash.Hash {
	var h hash.Hash
	b := rapid.SliceOfN(rapid.Byte(), len(h), len(h)).Draw(t, label)
	copy(h[:], b)
	// avoid zero-hash edge unless we explicitly want it
	allZero := true
	for _, x := range h[:] {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		h[0] = 1
	}
	return h
}

func drawNonEmptyBytes(t *rapid.T, label string, minLen, maxLen int) []byte {
	b := rapid.SliceOfN(rapid.Byte(), minLen, maxLen).Draw(t, label)
	if len(b) == 0 {
		t.Fatalf("generator bug: expected non-empty %s", label)
	}
	return b
}

func concat(chunks [][]byte) []byte {
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

func firstOccurrenceByIndex(in []SealedSlice) map[uint8]SealedSlice {
	firstByIdx := make(map[uint8]SealedSlice, len(in))
	for _, s := range in {
		if _, ok := firstByIdx[s.RSSliceIndex]; ok {
			continue
		}
		firstByIdx[s.RSSliceIndex] = s
	}
	return firstByIdx
}

func assertOutCountInRange(rt *rapid.T, got int, k, total uint8) {
	if got < int(k) || got > int(total) {
		rt.Fatalf(
			"unexpected output size: got=%d want in [%d,%d]",
			got,
			k,
			total,
		)
	}
}

func assertSliceMetadataConsistent(
	rt *rapid.T,
	s SealedSlice,
	chunkHash hash.Hash,
	k uint8,
	p uint8,
	total uint8,
) {
	if s.ChunkHash != chunkHash {
		rt.Fatalf("output contains mixed ChunkHash")
	}
	if s.RSDataSlices != k || s.RSParitySlices != p {
		rt.Fatalf(
			"output contains mixed RS params: got %d/%d want %d/%d",
			s.RSDataSlices,
			s.RSParitySlices,
			k,
			p,
		)
	}
	if len(s.Nonce) == 0 {
		rt.Fatalf("output slice has empty nonce")
	}
	if s.RSSliceIndex >= total {
		rt.Fatalf(
			"output index out of range: %d not in [0,%d)",
			s.RSSliceIndex,
			total,
		)
	}
}

func assertOutSortedUniqueConsistent(
	rt *rapid.T,
	out []SealedSlice,
	chunkHash hash.Hash,
	k uint8,
	p uint8,
	total uint8,
) {
	seen := map[uint8]bool{}
	prev := -1
	for _, s := range out {
		assertSliceMetadataConsistent(rt, s, chunkHash, k, p, total)
		if seen[s.RSSliceIndex] {
			rt.Fatalf("output contains duplicate index %d", s.RSSliceIndex)
		}
		seen[s.RSSliceIndex] = true

		if int(s.RSSliceIndex) <= prev {
			rt.Fatalf(
				"output not strictly increasing by index: prev=%d now=%d",
				prev,
				s.RSSliceIndex,
			)
		}
		prev = int(s.RSSliceIndex)
	}
}

func sealedSliceEqualOnSelectionFields(a, b SealedSlice) bool {
	return a.ChunkHash == b.ChunkHash &&
		a.RSDataSlices == b.RSDataSlices &&
		a.RSParitySlices == b.RSParitySlices &&
		a.RSSliceIndex == b.RSSliceIndex &&
		bytes.Equal(a.Nonce, b.Nonce)
}

func assertOutMatchesFirstByIndex(
	rt *rapid.T,
	out []SealedSlice,
	firstByIdx map[uint8]SealedSlice,
) {
	for _, s := range out {
		want, ok := firstByIdx[s.RSSliceIndex]
		if !ok {
			rt.Fatalf("output contains unexpected index %d", s.RSSliceIndex)
		}
		if !sealedSliceEqualOnSelectionFields(s, want) {
			rt.Fatalf(
				"output slice for idx=%d is not the first occurrence",
				s.RSSliceIndex,
			)
		}
	}
}

func makeValidSealedSliceForReconstruction(
	chunkHash hash.Hash,
	k uint8,
	p uint8,
	idx uint8,
	nonce []byte,
) SealedSlice {
	return SealedSlice{
		ChunkHash:      chunkHash,
		RSDataSlices:   k,
		RSParitySlices: p,
		RSSliceIndex:   idx,
		Nonce:          nonce,
	}
}

func buildReconstructionInputWithDuplicates(
	rt *rapid.T,
	chunkHash hash.Hash,
	k uint8,
	p uint8,
) []SealedSlice {
	total := k + p
	base := make([]SealedSlice, 0, int(k)+10)

	for idx := uint8(0); idx < k; idx++ {
		base = append(
			base,
			makeValidSealedSliceForReconstruction(
				chunkHash,
				k,
				p,
				idx,
				drawNonEmptyBytes(rt, "nonce_mandatory", 1, 32),
			),
		)
	}

	extras := rapid.IntRange(0, 30).Draw(rt, "extras")
	for i := 0; i < extras; i++ {
		idx := rapid.Uint8Range(0, total-1).Draw(rt, "idx_extra")
		base = append(
			base,
			makeValidSealedSliceForReconstruction(
				chunkHash,
				k,
				p,
				idx,
				drawNonEmptyBytes(rt, "nonce_extra", 1, 32),
			),
		)
	}

	return base
}

// --------- properties: chunker / compression ---------

func TestRapid_Chunker_RoundTripConcatenation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		payload := rapid.SliceOfN(rapid.Byte(), 0, 64*1024).Draw(rt, "payload")
		chunks, err := chunker(payload)
		if err != nil {
			rt.Fatalf("chunker error: %v", err)
		}
		got := concat(chunks)
		if !bytes.Equal(got, payload) {
			rt.Fatalf(
				"chunker roundtrip mismatch: len(got)=%d len(want)=%d",
				len(got),
				len(payload),
			)
		}
		if len(payload) > 0 {
			for i, c := range chunks {
				if len(c) == 0 {
					rt.Fatalf("chunk %d is empty for non-empty payload", i)
				}
			}
		}
	})
}

func TestRapid_CompressDecompress_RoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		clear := rapid.SliceOfN(rapid.Byte(), 0, 256*1024).Draw(rt, "clear")
		comp, err := compressChunk(clear)
		if err != nil {
			rt.Fatalf("compressChunk error: %v", err)
		}
		got, err := decompressChunkData(comp)
		if err != nil {
			rt.Fatalf("decompressChunkData error: %v", err)
		}
		if !bytes.Equal(got, clear) {
			rt.Fatalf(
				"compress/decompress mismatch: len(got)=%d len(want)=%d",
				len(got),
				len(clear),
			)
		}
	})
}

// --------- properties: validateShardCount ---------

func TestRapid_ValidateShardCount_MatchesSpec(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		k := rapid.Uint8Min(1).Draw(rt, "k")             // k>0
		p := rapid.Uint8Range(0, 255).Draw(rt, "p")      // p>=0
		shardLen := rapid.IntRange(0, 400).Draw(rt, "n") // can exceed limits

		shards := make([][]byte, shardLen)
		err := validateShardCount(shards, k, p)

		wantErr := shardLen > int(k)+int(p) || shardLen > maxSliceCount
		if wantErr && err == nil {
			rt.Fatalf("expected error for n=%d k=%d p=%d, got nil", shardLen, k, p)
		}
		if !wantErr && err != nil {
			rt.Fatalf("unexpected error for n=%d k=%d p=%d: %v", shardLen, k, p, err)
		}
	})
}

// --------- properties: selectSealedSlicesForReconstruction (valid inputs)
// ---------

func TestRapid_SelectSealedSlicesForReconstruction_ValidInputInvariants(
	t *testing.T,
) {
	rapid.Check(t, rapidSelectSealedSlicesForReconstruction_ValidInputInvariants)
}

func rapidSelectSealedSlicesForReconstruction_ValidInputInvariants(
	rt *rapid.T,
) {
	chunkHash := drawHash(rt, "chunkHash")
	k := rapid.Uint8Range(1, 16).Draw(rt, "k")
	p := rapid.Uint8Range(0, 16).Draw(rt, "p")

	base := buildReconstructionInputWithDuplicates(rt, chunkHash, k, p)
	in := rapid.Permutation(base).Draw(rt, "in")

	out, err := selectSealedSlicesForReconstruction(in)
	if err != nil {
		rt.Fatalf("unexpected error on valid input: %v", err)
	}

	total := k + p
	assertOutCountInRange(rt, len(out), k, total)
	firstByIdx := firstOccurrenceByIndex(in)
	assertOutSortedUniqueConsistent(rt, out, chunkHash, k, p, total)
	assertOutMatchesFirstByIndex(rt, out, firstByIdx)
}

// --------- properties: selectSealedSlicesForReconstruction (error cases)
// ---------

func TestRapid_SelectSealedSlicesForReconstruction_ErrorConditions(
	t *testing.T,
) {
	type errCase struct {
		name  string
		build func(*rapid.T) []SealedSlice
	}
	cases := []errCase{
		{name: "empty", build: buildSelectSealedSlicesErrEmpty},
		{name: "mixedChunk", build: buildSelectSealedSlicesErrMixedChunk},
		{name: "mixedRS", build: buildSelectSealedSlicesErrMixedRS},
		{name: "outOfRangeIndex", build: buildSelectSealedSlicesErrOutOfRangeIndex},
		{name: "emptyNonce", build: buildSelectSealedSlicesErrEmptyNonce},
		{
			name:  "notEnoughDistinct",
			build: buildSelectSealedSlicesErrNotEnoughDistinct,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rapid.Check(t, func(rt *rapid.T) {
				in := tc.build(rt)
				_, err := selectSealedSlicesForReconstruction(in)
				if err == nil {
					rt.Fatalf("expected error for case %q, got nil", tc.name)
				}
			})
		})
	}
}

func drawSelectSealedSlicesParams(
	rt *rapid.T,
	minK uint8,
) (chunkHash hash.Hash, k, p, total uint8) {
	chunkHash = drawHash(rt, "chunkHash")
	k = rapid.Uint8Range(minK, 16).Draw(rt, "k")
	p = rapid.Uint8Range(0, 16).Draw(rt, "p")
	total = k + p
	return chunkHash, k, p, total
}

func makeMinimalValidSliceForSelectSealedSlices(
	chunkHash hash.Hash,
	k uint8,
	p uint8,
	idx uint8,
) SealedSlice {
	return makeValidSealedSliceForReconstruction(chunkHash, k, p, idx, []byte{1})
}

func buildSelectSealedSlicesErrEmpty(rt *rapid.T) []SealedSlice {
	_, _, _, _ = drawSelectSealedSlicesParams(rt, 1)
	return nil
}

func buildSelectSealedSlicesErrMixedChunk(rt *rapid.T) []SealedSlice {
	chunkHash, k, p, _ := drawSelectSealedSlicesParams(rt, 1)
	in := []SealedSlice{
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 1),
	}
	in[1].ChunkHash = drawHash(rt, "otherChunk")
	return in
}

func buildSelectSealedSlicesErrMixedRS(rt *rapid.T) []SealedSlice {
	chunkHash, k, p, _ := drawSelectSealedSlicesParams(rt, 1)
	in := []SealedSlice{
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 1),
	}
	in[1].RSParitySlices++
	return in
}

func buildSelectSealedSlicesErrOutOfRangeIndex(rt *rapid.T) []SealedSlice {
	chunkHash, k, p, total := drawSelectSealedSlicesParams(rt, 1)
	in := []SealedSlice{
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 1),
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
	}
	in[2].RSSliceIndex = total
	return in
}

func buildSelectSealedSlicesErrEmptyNonce(rt *rapid.T) []SealedSlice {
	chunkHash, k, p, _ := drawSelectSealedSlicesParams(rt, 1)
	in := []SealedSlice{
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 1),
	}
	in[1].Nonce = nil
	return in
}

func buildSelectSealedSlicesErrNotEnoughDistinct(rt *rapid.T) []SealedSlice {
	chunkHash, k, p, _ := drawSelectSealedSlicesParams(rt, 2)
	return []SealedSlice{
		makeMinimalValidSliceForSelectSealedSlices(chunkHash, k, p, 0),
	}
}

// --------- properties: SealedSlice hashing ---------

func TestRapid_SealedSliceHash_StableAndSensitiveToRelevantFields(
	t *testing.T,
) {
	rapid.Check(t, rapidSealedSliceHashStableAndSensitiveToRelevantFields)
}

func rapidSealedSliceHashStableAndSensitiveToRelevantFields(rt *rapid.T) {
	ctx := context.Background()
	base := drawSealedSliceForHashChecks(rt)

	h1 := mustGetSealedSliceHash(rt, ctx, &base, "GetHash")
	h2 := mustGetSealedSliceHash(rt, ctx, &base, "GetHash again")
	if h1 != h2 {
		rt.Fatalf("hash not stable across repeated GetHash calls")
	}

	h3 := mustValidateSealedSliceHash(rt, ctx, &base, "ValidateHash")
	if h1 != h3 {
		rt.Fatalf("ValidateHash produced different hash")
	}

	assertNonHashFieldDoesNotAffectHashValidation(rt, ctx, base)

	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"ChunkHash",
		func(s *SealedSlice) { mutateChunkHash(rt, s) },
	)
	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"RSDataSlices",
		mutateRSDataSlices,
	)
	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"RSParitySlices",
		mutateRSParitySlices,
	)
	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"RSSliceIndex",
		mutateRSSliceIndex,
	)
	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"Nonce",
		mutateNonce,
	)
	assertHashSensitiveToMutation(
		rt,
		ctx,
		base,
		h1,
		"Payload",
		mutateSealedPayload,
	)
}

func drawSealedSliceForHashChecks(rt *rapid.T) SealedSlice {
	base := SealedSlice{
		ChunkHash:    drawHash(rt, "chunkHash"),
		RSDataSlices: rapid.Uint8Range(1, 32).Draw(rt, "k"),
		RSParitySlices: rapid.Uint8Range(
			1,
			32,
		).Draw(
			rt,
			"p",
		), // GetHash requires >0
		RSSliceIndex:  rapid.Uint8().Draw(rt, "idx"),
		Nonce:         drawNonEmptyBytes(rt, "nonce", 1, 32),
		sealedPayload: drawNonEmptyBytes(rt, "payload", 1, 512),
	}

	total := base.RSDataSlices + base.RSParitySlices
	base.RSSliceIndex = base.RSSliceIndex % total

	return base
}

func mustGetSealedSliceHash(
	rt *rapid.T,
	ctx context.Context,
	s *SealedSlice,
	label string,
) hash.Hash {
	h, err := s.GetHash(ctx)
	if err != nil {
		rt.Fatalf("%s error: %v", label, err)
	}
	return h
}

func mustValidateSealedSliceHash(
	rt *rapid.T,
	ctx context.Context,
	s *SealedSlice,
	label string,
) hash.Hash {
	h, err := s.ValidateHash(ctx)
	if err != nil {
		rt.Fatalf("%s error: %v", label, err)
	}
	return h
}

func assertNonHashFieldDoesNotAffectHashValidation(
	rt *rapid.T,
	ctx context.Context,
	base SealedSlice,
) {
	base2 := base
	base2.cas = &CAS{}
	if _, err := base2.ValidateHash(ctx); err != nil {
		rt.Fatalf("ValidateHash failed after non-hash change: %v", err)
	}
}

func requireValidateHashFails(
	rt *rapid.T,
	ctx context.Context,
	s SealedSlice,
	name string,
) {
	if _, err := s.ValidateHash(ctx); err == nil {
		rt.Fatalf("ValidateHash unexpectedly succeeded after mutating %s", name)
	}
}

func assertHashSensitiveToMutation(
	rt *rapid.T,
	ctx context.Context,
	base SealedSlice,
	orig hash.Hash,
	name string,
	mutate func(*SealedSlice),
) {
	mod := base
	mutate(&mod)
	requireValidateHashFails(rt, ctx, mod, name)

	mod.Hash = hash.Hash{}
	newH := mustGetSealedSliceHash(
		rt,
		ctx,
		&mod,
		"GetHash after mutating "+name,
	)
	if newH == orig {
		rt.Fatalf("hash did not change after mutating %s", name)
	}
}

func mutateChunkHash(rt *rapid.T, s *SealedSlice) {
	h := drawHash(rt, "otherChunk")
	if h == s.ChunkHash {
		h[0] ^= 0xFF
	}
	s.ChunkHash = h
}

func mutateRSDataSlices(s *SealedSlice) {
	s.RSDataSlices++
}

func mutateRSParitySlices(s *SealedSlice) {
	s.RSParitySlices++
}

func mutateRSSliceIndex(s *SealedSlice) {
	s.RSSliceIndex++
}

func mutateNonce(s *SealedSlice) {
	s.Nonce = append([]byte(nil), s.Nonce...)
	s.Nonce[0] ^= 0xFF
}

func mutateSealedPayload(s *SealedSlice) {
	s.sealedPayload = append([]byte(nil), s.sealedPayload...)
	s.sealedPayload[0] ^= 0xFF
}

// --------- heavier property: storeSealedSlicesFromChunk ---------
// This exercises compression + RS + encryption + hash + persistence invariants.
// Skip in -short mode to keep CI snappy 🧯

func TestRapid_StoreSealedSlicesFromChunk_PersistsAndValidates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rapid sealed-slice pipeline test in -short mode")
	}

	rapid.Check(t, rapidStoreSealedSlicesFromChunkPersistsAndValidates)
}

func rapidStoreSealedSlicesFromChunkPersistsAndValidates(rt *rapid.T) {
	ctx := context.Background()
	clear := drawNonEmptyBytes(rt, "clearChunk", 1, 64*1024)
	chunkHash := hash.HashBytes(clear)

	rsK := rapid.Uint8Range(1, 16).Draw(rt, "rsK")
	rsP := rapid.Uint8Range(1, 16).Draw(rt, "rsP")

	dr := newFakeDataRouter(nil)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	opts := storeSealedSlicesFromChunkOpts{
		CAS:             cas,
		Crypt:           *c,
		ClearChunkBytes: clear,
		ChunkHash:       chunkHash,
		RSDataSlices:    rsK,
		RSParitySlices:  rsP,
	}
	if err := opts.Validate(); err != nil {
		rt.Fatalf("opts.Validate unexpected error: %v", err)
	}

	slices, err := storeSealedSlicesFromChunk(ctx, opts)
	if err != nil {
		rt.Fatalf("storeSealedSlicesFromChunk error: %v", err)
	}

	wantTotal := int(rsK) + int(rsP)
	assertStoreSealedSlicesCounts(rt, slices, dr, ki, wantTotal)
	assertStoredSealedSlicesValid(rt, ctx, slices, chunkHash, rsK, rsP)
	assertStoredSealedSlicesIndicesCoverRange(rt, slices, wantTotal)
}

func assertStoreSealedSlicesCounts(
	rt *rapid.T,
	slices []SealedSlice,
	dr *fakeDataRouter,
	ki *fakeKeyIndex,
	wantTotal int,
) {
	if len(slices) != wantTotal {
		rt.Fatalf("unexpected slice count: got=%d want=%d", len(slices), wantTotal)
	}
	if dr.NumSealedSlices() != wantTotal {
		rt.Fatalf(
			"router sealed slice count mismatch: got=%d want=%d",
			dr.NumSealedSlices(),
			wantTotal,
		)
	}
	if ki.NumSecrets() != wantTotal {
		rt.Fatalf(
			"keyIndex secret count mismatch: got=%d want=%d",
			ki.NumSecrets(),
			wantTotal,
		)
	}
}

func assertStoredSealedSliceMetadata(
	rt *rapid.T,
	s SealedSlice,
	chunkHash hash.Hash,
	rsK uint8,
	rsP uint8,
) {
	if s.ChunkHash != chunkHash {
		rt.Fatalf("slice has wrong ChunkHash")
	}
	if s.RSDataSlices != rsK || s.RSParitySlices != rsP {
		rt.Fatalf("slice has wrong RS params")
	}
	if len(s.Nonce) == 0 {
		rt.Fatalf("slice has empty nonce")
	}
	if s.Hash == (hash.Hash{}) {
		rt.Fatalf("slice hash is zero")
	}
}

func assertStoredSealedSlicesValid(
	rt *rapid.T,
	ctx context.Context,
	slices []SealedSlice,
	chunkHash hash.Hash,
	rsK uint8,
	rsP uint8,
) {
	seen := map[uint8]bool{}
	for _, s := range slices {
		assertStoredSealedSliceMetadata(rt, s, chunkHash, rsK, rsP)
		if seen[s.RSSliceIndex] {
			rt.Fatalf("duplicate RSSliceIndex %d", s.RSSliceIndex)
		}
		seen[s.RSSliceIndex] = true

		if _, err := s.ValidateHash(ctx); err != nil {
			rt.Fatalf("ValidateHash failed for idx=%d: %v", s.RSSliceIndex, err)
		}
	}
}

func assertStoredSealedSlicesIndicesCoverRange(
	rt *rapid.T,
	slices []SealedSlice,
	wantTotal int,
) {
	idxs := make([]int, 0, len(slices))
	for _, s := range slices {
		idxs = append(idxs, int(s.RSSliceIndex))
	}

	sort.Ints(idxs)
	for i := 0; i < wantTotal; i++ {
		if idxs[i] != i {
			rt.Fatalf("expected indices 0..%d, got %v", wantTotal-1, idxs)
		}
	}
}

// --------- basic rapid roundtrip over StoreBlob / GetContent ---------

func TestRapid_CAS_StoreBlob_RoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rapid blob roundtrip in -short mode")
	}

	rapid.Check(t, func(rt *rapid.T) {
		ctx := context.Background()

		payload := drawNonEmptyBytes(rt, "payload", 1, 256*1024)
		chunkSizeMap := buildChunkSizeMap(t, payload)

		dr := newFakeDataRouter(chunkSizeMap)
		ki := newFakeKeyIndex()
		c := crypt.New()
		cas := NewCAS(dr, ki)

		created := time.Now().UnixMilli()
		_, err := cas.StoreBlob(ctx, payload, "rapid/key", hash.Hash{}, created, *c)
		if err != nil {
			rt.Fatalf("StoreBlob error: %v", err)
		}

		blob := firstStoredBlob(t, dr)
		got, err := blob.GetContent(ctx, *c)
		if err != nil {
			rt.Fatalf("GetContent error: %v", err)
		}

		if !bytes.Equal(got, payload) {
			rt.Fatalf(
				"roundtrip mismatch: len(got)=%d len(want)=%d",
				len(got),
				len(payload),
			)
		}
	})
}
