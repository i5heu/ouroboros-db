package cas

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// fakeDataRouter is a strict in-memory dataRouter for tests; it fails fast when
// lookups miss and exposes copies for assertions.
type fakeDataRouter struct {
	blobs          map[hash.Hash]Blob
	chunks         map[hash.Hash]Chunk
	sealedSlices   map[hash.Hash]SealedSlice
	sealedPayloads map[hash.Hash][]byte
	chunkSealed    map[hash.Hash][]SealedSlice
	chunkSizes     map[hash.Hash]int

	pendingChunkHashes []hash.Hash
}

func newFakeDataRouter(chunkSizes map[hash.Hash]int) *fakeDataRouter {
	copiedChunkSizes := make(map[hash.Hash]int, len(chunkSizes))
	for h, size := range chunkSizes {
		copiedChunkSizes[h] = size
	}

	return &fakeDataRouter{
		blobs:          make(map[hash.Hash]Blob),
		chunks:         make(map[hash.Hash]Chunk),
		sealedSlices:   make(map[hash.Hash]SealedSlice),
		sealedPayloads: make(map[hash.Hash][]byte),
		chunkSealed:    make(map[hash.Hash][]SealedSlice),
		chunkSizes:     copiedChunkSizes,
	}
}

func (f *fakeDataRouter) GetBlob(
	ctx context.Context,
	h hash.Hash,
) (Blob, error) {
	if blob, ok := f.blobs[h]; ok {
		return f.cloneBlob(blob), nil
	}
	return Blob{}, fmt.Errorf("blob %s not found", h.String())
}

func (f *fakeDataRouter) SetBlob(ctx context.Context, blob Blob) error {
	chunkHashes := blob.chunks
	if len(chunkHashes) == 0 {
		if len(f.pendingChunkHashes) == 0 {
			return fmt.Errorf("no pending chunks available for blob %s", blob.Key)
		}
		chunkHashes = f.pendingChunkHashes
	}

	blob.chunks = append([]hash.Hash(nil), chunkHashes...)
	blob.Hash = computeFakeBlobHash(
		blob.Key,
		blob.Parent,
		uint64( //nolint:gosec // created is a non-negative timestamp in tests
			blob.Created,
		),
		blob.chunks,
	)

	if _, exists := f.blobs[blob.Hash]; exists {
		return fmt.Errorf("blob %s already stored", blob.Hash.String())
	}

	f.blobs[blob.Hash] = blob
	f.pendingChunkHashes = nil
	return nil
}

func (f *fakeDataRouter) GetChunk(
	ctx context.Context,
	h hash.Hash,
) (Chunk, error) {
	if chunk, ok := f.chunks[h]; ok {
		return f.cloneChunk(chunk), nil
	}
	return Chunk{}, fmt.Errorf("chunk %s not found", h.String())
}

func (f *fakeDataRouter) SetChunk(ctx context.Context, chunk Chunk) error {
	if chunk.Hash == (hash.Hash{}) {
		return fmt.Errorf("chunk hash must be set")
	}

	if len(f.chunkSizes) > 0 {
		size, ok := f.chunkSizes[chunk.Hash]
		if !ok {
			return fmt.Errorf("chunk size for %s not provided", chunk.Hash.String())
		}
		chunk.Size = size
	}
	f.chunks[chunk.Hash] = chunk
	f.pendingChunkHashes = append(f.pendingChunkHashes, chunk.Hash)
	return nil
}

func (f *fakeDataRouter) GetSealedSlicesForChunk(
	ctx context.Context,
	chunkHash hash.Hash,
) ([]SealedSlice, error) {
	slices, ok := f.chunkSealed[chunkHash]
	if !ok {
		return nil, fmt.Errorf("no sealed slices for chunk %s", chunkHash.String())
	}
	return f.cloneSealedSliceSlice(slices), nil
}

func (f *fakeDataRouter) GetSealedSlice(
	ctx context.Context,
	h hash.Hash,
) (SealedSlice, error) {
	if slice, ok := f.sealedSlices[h]; ok {
		return f.cloneSealedSlice(slice), nil
	}
	return SealedSlice{}, fmt.Errorf("sealed slice %s not found", h.String())
}

func (f *fakeDataRouter) SetSealedSlice(
	ctx context.Context,
	slice SealedSlice,
) error {
	if slice.Hash == (hash.Hash{}) {
		return fmt.Errorf("sealed slice hash must be set")
	}
	if slice.ChunkHash == (hash.Hash{}) {
		return fmt.Errorf("sealed slice %s missing chunk hash", slice.Hash.String())
	}
	if len(slice.sealedPayload) == 0 {
		return fmt.Errorf("sealed slice %s missing payload", slice.Hash.String())
	}

	cloned := f.cloneSealedSlice(slice)
	f.sealedSlices[cloned.Hash] = cloned
	f.sealedPayloads[cloned.Hash] = cloned.sealedPayload
	f.chunkSealed[cloned.ChunkHash] = append(
		f.chunkSealed[cloned.ChunkHash],
		cloned,
	)
	return nil
}

func (f *fakeDataRouter) GetSealedSlicePayload(
	ctx context.Context,
	h hash.Hash,
) ([]byte, error) {
	payload, ok := f.sealedPayloads[h]
	if !ok {
		return nil, fmt.Errorf("sealed slice payload %s not found", h.String())
	}
	return append([]byte(nil), payload...), nil
}

func (f *fakeDataRouter) NumBlobs() int {
	return len(f.blobs)
}

func (f *fakeDataRouter) NumChunks() int {
	return len(f.chunks)
}

func (f *fakeDataRouter) NumSealedSlices() int {
	return len(f.sealedSlices)
}

func (f *fakeDataRouter) BlobsByHash() map[hash.Hash]Blob {
	result := make(map[hash.Hash]Blob, len(f.blobs))
	for h, blob := range f.blobs {
		result[h] = f.cloneBlob(blob)
	}
	return result
}

func (f *fakeDataRouter) ChunksByHash() map[hash.Hash]Chunk {
	result := make(map[hash.Hash]Chunk, len(f.chunks))
	for h, chunk := range f.chunks {
		result[h] = f.cloneChunk(chunk)
	}
	return result
}

func (f *fakeDataRouter) SealedSlicesByHash() map[hash.Hash]SealedSlice {
	result := make(map[hash.Hash]SealedSlice, len(f.sealedSlices))
	for h, slice := range f.sealedSlices {
		result[h] = f.cloneSealedSlice(slice)
	}
	return result
}

func (f *fakeDataRouter) SealedSlicesByChunk() map[hash.Hash][]SealedSlice {
	result := make(map[hash.Hash][]SealedSlice, len(f.chunkSealed))
	for chunkHash, slices := range f.chunkSealed {
		result[chunkHash] = f.cloneSealedSliceSlice(slices)
	}
	return result
}

func (f *fakeDataRouter) SealedPayloads() map[hash.Hash][]byte {
	result := make(map[hash.Hash][]byte, len(f.sealedPayloads))
	for h, payload := range f.sealedPayloads {
		result[h] = append([]byte(nil), payload...)
	}
	return result
}

func (f *fakeDataRouter) ReplaceChunkSealedSlices(
	chunkHash hash.Hash,
	slices []SealedSlice,
) error {
	if chunkHash == (hash.Hash{}) {
		return fmt.Errorf("chunk hash must be provided to replace slices")
	}

	// Drop existing entries for this chunk to keep the fake consistent.
	for h, slice := range f.sealedSlices {
		if slice.ChunkHash == chunkHash {
			delete(f.sealedSlices, h)
			delete(f.sealedPayloads, h)
		}
	}

	cloned := f.cloneSealedSliceSlice(slices)
	f.chunkSealed[chunkHash] = cloned
	for _, slice := range cloned {
		f.sealedSlices[slice.Hash] = slice
		if len(slice.sealedPayload) > 0 {
			f.sealedPayloads[slice.Hash] = slice.sealedPayload
		}
	}
	return nil
}

func (f *fakeDataRouter) cloneBlob(blob Blob) Blob {
	blob.chunks = append([]hash.Hash(nil), blob.chunks...)
	return blob
}

func (f *fakeDataRouter) cloneChunk(chunk Chunk) Chunk {
	if chunk.content != nil {
		chunk.content = append([]byte(nil), chunk.content...)
	}
	return chunk
}

func (f *fakeDataRouter) cloneSealedSlice(slice SealedSlice) SealedSlice {
	slice.Nonce = append([]byte(nil), slice.Nonce...)
	slice.sealedPayload = append([]byte(nil), slice.sealedPayload...)
	return slice
}

func (f *fakeDataRouter) cloneSealedSliceSlice(
	slices []SealedSlice,
) []SealedSlice {
	out := make([]SealedSlice, len(slices))
	for i, slice := range slices {
		out[i] = f.cloneSealedSlice(slice)
	}
	return out
}

// fakeKeyIndex is an in-memory key index for tests that errors on missing data.
type fakeKeyIndex struct {
	secrets map[string][]byte
}

func newFakeKeyIndex() *fakeKeyIndex {
	return &fakeKeyIndex{
		secrets: make(map[string][]byte),
	}
}

func (f *fakeKeyIndex) key(sliceHash, pubHash hash.Hash) string {
	return sliceHash.String() + "|" + pubHash.String()
}

func (f *fakeKeyIndex) Get(
	sealedSliceHash, pubKeyHash hash.Hash,
) ([][]byte, error) {
	key := f.key(sealedSliceHash, pubKeyHash)
	secret, ok := f.secrets[key]
	if !ok {
		return nil, fmt.Errorf("secret not found for %s", key)
	}
	return [][]byte{append([]byte(nil), secret...)}, nil
}

func (f *fakeKeyIndex) Set(
	sealedSliceHash, pubKeyHash hash.Hash,
	encapsulatedSecret []byte,
) error {
	key := f.key(sealedSliceHash, pubKeyHash)
	if len(encapsulatedSecret) == 0 {
		return fmt.Errorf("encapsulated secret for %s must not be empty", key)
	}
	f.secrets[key] = encapsulatedSecret
	return nil
}

func (f *fakeKeyIndex) NumSecrets() int {
	return len(f.secrets)
}

func (f *fakeKeyIndex) Secrets() map[string][]byte {
	result := make(map[string][]byte, len(f.secrets))
	for k, secret := range f.secrets {
		result[k] = append([]byte(nil), secret...)
	}
	return result
}

func computeFakeBlobHash(
	key string,
	parent hash.Hash,
	created uint64,
	chunks []hash.Hash,
) hash.Hash {
	var createdBytes [8]byte
	binary.LittleEndian.PutUint64(createdBytes[:], created)

	buf := make([]byte, 0, len(key)+len(parent)+8+len(chunks)*len(hash.Hash{}))
	buf = append(buf, []byte(key)...)
	buf = append(buf, parent[:]...)
	buf = append(buf, createdBytes[:]...)
	for _, ch := range chunks {
		buf = append(buf, ch[:]...)
	}
	return hash.HashBytes(buf)
}

func firstStoredBlob(tb testing.TB, dr *fakeDataRouter) Blob {
	tb.Helper()

	for _, blob := range dr.BlobsByHash() {
		if blob.Hash == (hash.Hash{}) {
			tb.Fatalf("stored blob missing hash")
		}
		if len(blob.chunks) == 0 {
			tb.Fatalf(
				"stored blob %s missing chunk hashes",
				blob.Hash.String(),
			)
		}
		return blob
	}
	tb.Fatalf("no blob stored in fake data router")
	return Blob{}
}

func buildChunkSizeMap(tb testing.TB, payload []byte) map[hash.Hash]int {
	tb.Helper()

	chunks, err := chunker(payload)
	if err != nil {
		tb.Fatalf("failed to chunk payload: %v", err)
	}
	if len(chunks) == 0 {
		tb.Fatalf(
			"chunker returned no chunks for payload length %d",
			len(payload),
		)
	}

	sizeMap := make(map[hash.Hash]int)
	for _, ch := range chunks {
		compressed, err := compressChunk(ch)
		if err != nil {
			tb.Fatalf("failed to compress chunk: %v", err)
		}
		sizeMap[hash.HashBytes(ch)] = len(compressed)
	}
	return sizeMap
}

func storeBlobAndSelectChunk(
	t *testing.T,
	payload []byte,
) (context.Context, *crypt.Crypt, *fakeDataRouter, Chunk) {
	t.Helper()

	ctx := context.Background()
	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()
	if _, err := cas.StoreBlob(
		ctx,
		payload,
		"test/key",
		hash.Hash{},
		created,
		*c,
	); err != nil {
		t.Fatalf("StoreBlob returned error: %v", err)
	}

	if dr.NumChunks() == 0 {
		t.Fatalf("no chunks stored for payload length %d", len(payload))
	}

	var chunk Chunk
	for _, ch := range dr.ChunksByHash() {
		chunk = ch
		break
	}

	chunk.cas = cas // ensure cas pointer is present in copy retrieved from map
	slices, err := dr.GetSealedSlicesForChunk(ctx, chunk.Hash)
	if err != nil {
		t.Fatalf(
			"failed to get sealed slices for chunk %s: %v",
			chunk.Hash.String(),
			err,
		)
	}
	if len(slices) == 0 {
		t.Fatalf("no sealed slices stored for chunk %s", chunk.Hash.String())
	}
	return ctx, c, dr, chunk
}

func FuzzCAS_StoreBlob_RoundTrip(t *testing.F) {
	ctx := context.Background()

	t.Add([]byte("fuzz-seed"))
	t.Add(bytes.Repeat([]byte("ouroboros"), 16))

	t.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			t.Skip("empty payload not interesting")
		}

		chunkSizeMap := buildChunkSizeMap(t, payload)
		dr := newFakeDataRouter(chunkSizeMap)
		ki := newFakeKeyIndex()
		c := crypt.New()
		cas := NewCAS(dr, ki)

		created := time.Now().UnixMilli()
		_, err := cas.StoreBlob(
			ctx,
			payload,
			"fuzz/key",
			hash.Hash{},
			created,
			*c,
		)
		if err != nil {
			t.Fatalf("StoreBlob returned error: %v", err)
		}

		storedBlob := firstStoredBlob(t, dr)
		content, err := storedBlob.GetContent(ctx, *c)
		if err != nil {
			t.Fatalf("GetContent returned error: %v", err)
		}
		if !bytes.Equal(content, payload) {
			t.Fatalf(
				"round trip mismatch: expected %d bytes, got %d",
				len(payload),
				len(content),
			)
		}
	})
}

func TestCAS_StoreBlob_RoundTripPersistsAndRestoresContent(t *testing.T) {
	ctx := context.Background()
	payload := bytes.Repeat([]byte("ouroboros-data-"), 4096)

	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()
	_, err := cas.StoreBlob(ctx, payload, "test/key", hash.Hash{}, created, *c)
	if err != nil {
		t.Fatalf("StoreBlob returned error: %v", err)
	}
	blob := firstStoredBlob(t, dr)

	t.Run("returnsOriginalContent", func(t *testing.T) {
		content, err := blob.GetContent(ctx, *c)
		if err != nil {
			t.Fatalf("GetContent returned error: %v", err)
		}
		if !bytes.Equal(content, payload) {
			t.Fatalf(
				"blob content mismatch: expected %d bytes, got %d",
				len(payload),
				len(content),
			)
		}
	})

	t.Run("storesChunksWithEnoughSlices", func(t *testing.T) {
		if dr.NumChunks() == 0 {
			t.Fatalf(
				"expected at least one chunk to be stored, got %d",
				dr.NumChunks(),
			)
		}
		for chunkHash, slices := range dr.SealedSlicesByChunk() {
			if len(slices) == 0 {
				t.Fatalf(
					"expected sealed slices for chunk %s, got none",
					chunkHash.String(),
				)
			}
			if len(slices) < int(slices[0].RSDataSlices) {
				t.Fatalf(
					"chunk %s has insufficient slices: have %d, need %d",
					chunkHash.String(),
					len(slices),
					slices[0].RSDataSlices,
				)
			}
		}
	})
}

func TestCAS_StoreBlob_ErrorScenarios(t *testing.T) {
	ctx := context.Background()
	c := crypt.New()
	dr := newFakeDataRouter(nil)
	ki := newFakeKeyIndex()
	cas := NewCAS(dr, ki)

	tests := []struct {
		name    string
		payload []byte
		created int64
	}{
		{name: "nilPayload", payload: nil, created: time.Now().UnixMilli()},
		{name: "invalidTimestamp", payload: []byte("data"), created: 0},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Helper()
			_, err := cas.StoreBlob(
				ctx,
				tc.payload,
				"key",
				hash.Hash{},
				tc.created,
				*c,
			)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestCAS_storeChunksFromBlob_RequiresRouterAndKeyIndex(t *testing.T) {
	ctx := context.Background()
	c := crypt.New()
	data := []byte("data")

	_, err := (&CAS{}).storeChunksFromBlob(ctx, data, *c)
	if err == nil || err.Error() != "dataRouter is nil" {
		t.Fatalf("expected dataRouter nil error, got %v", err)
	}

	dr := newFakeDataRouter(nil)
	_, err = (&CAS{dr: dr}).storeChunksFromBlob(ctx, data, *c)
	if err == nil || err.Error() != "keyIndex is nil" {
		t.Fatalf("expected keyIndex nil error, got %v", err)
	}
}

func TestStoreSealedSlicesFromChunkOpts_ValidateRequirements(t *testing.T) {
	c := crypt.New()
	validOpts := storeSealedSlicesFromChunkOpts{
		CAS:             &CAS{},
		Crypt:           *c,
		ClearChunkBytes: []byte("chunk"),
		ChunkHash:       hash.HashBytes([]byte("chunk")),
		RSDataSlices:    2,
		RSParitySlices:  1,
	}
	if err := validOpts.Validate(); err != nil {
		t.Fatalf("expected valid options, got %v", err)
	}

	testCases := []storeSealedSlicesFromChunkOpts{
		{},
		{CAS: &CAS{}},
		{CAS: &CAS{}, Crypt: *c},
		{CAS: &CAS{}, Crypt: *c, ClearChunkBytes: []byte("chunk")},
		{
			CAS:             &CAS{},
			Crypt:           *c,
			ClearChunkBytes: []byte("chunk"),
			ChunkHash:       hash.HashBytes([]byte("chunk")),
		},
		{
			CAS:             &CAS{},
			Crypt:           *c,
			ClearChunkBytes: []byte("chunk"),
			ChunkHash:       hash.HashBytes([]byte("chunk")),
			RSDataSlices:    1,
		},
	}
	for i, tc := range testCases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error, got nil", i)
		}
	}
}

func TestSelectSealedSlicesForReconstruction_DistinctSlicesReturned(
	t *testing.T,
) {
	chunkHash := hash.HashBytes([]byte("chunk"))
	makeSlice := func(idx uint8) SealedSlice {
		return SealedSlice{
			ChunkHash:      chunkHash,
			RSDataSlices:   2,
			RSParitySlices: 1,
			RSSliceIndex:   idx,
			Nonce:          []byte{1, 2, 3},
		}
	}

	slices := []SealedSlice{
		makeSlice(0),
		makeSlice(1),
		makeSlice(1), // duplicate should be ignored
	}

	selected, err := selectSealedSlicesForReconstruction(slices)
	if err != nil {
		t.Fatalf("expected selection to succeed: %v", err)
	}
	if len(selected) != 2 {
		t.Fatalf("expected 2 distinct slices, got %d", len(selected))
	}
	if selected[0].RSSliceIndex != 0 || selected[1].RSSliceIndex != 1 {
		t.Fatalf(
			"slices returned out of order or incorrect: got indices %d and %d",
			selected[0].RSSliceIndex,
			selected[1].RSSliceIndex,
		)
	}
}

func TestSelectSealedSlicesForReconstruction_InvalidInputs_Error(t *testing.T) {
	chunkHash := hash.HashBytes([]byte("chunk"))
	makeSlice := func(idx uint8) SealedSlice {
		return SealedSlice{
			ChunkHash:      chunkHash,
			RSDataSlices:   2,
			RSParitySlices: 1,
			RSSliceIndex:   idx,
			Nonce:          []byte{1, 2, 3},
		}
	}

	tests := []struct {
		name   string
		slices []SealedSlice
	}{
		{name: "empty", slices: nil},
		{
			name: "mixed chunk hashes",
			slices: []SealedSlice{
				makeSlice(0),
				{
					ChunkHash:      hash.HashBytes([]byte("other")),
					RSDataSlices:   2,
					RSParitySlices: 1,
					RSSliceIndex:   1,
					Nonce:          []byte{1},
				},
			},
		},
		{
			name: "mixed RS params",
			slices: []SealedSlice{
				makeSlice(0),
				{
					ChunkHash:      chunkHash,
					RSDataSlices:   3,
					RSParitySlices: 1,
					RSSliceIndex:   1,
					Nonce:          []byte{1},
				},
			},
		},
		{
			name: "index out of range",
			slices: []SealedSlice{
				makeSlice(0),
				{
					ChunkHash:      chunkHash,
					RSDataSlices:   2,
					RSParitySlices: 1,
					RSSliceIndex:   5,
					Nonce:          []byte{1},
				},
			},
		},
		{
			name: "not enough slices",
			slices: []SealedSlice{
				makeSlice(0),
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := selectSealedSlicesForReconstruction(tc.slices); err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
		})
	}
}

func TestSealedSlice_ValidateHash_DetectsTampering(t *testing.T) {
	chunkHash := hash.HashBytes([]byte("chunk"))
	s := SealedSlice{
		ChunkHash:      chunkHash,
		RSDataSlices:   1,
		RSParitySlices: 1,
		RSSliceIndex:   0,
		Nonce:          []byte{1, 2, 3},
		sealedPayload:  []byte{4, 5, 6},
	}

	hash1, err := s.GetHash(context.Background())
	if err != nil {
		t.Fatalf("GetHash returned error: %v", err)
	}

	hash2, err := s.ValidateHash(context.Background())
	if err != nil {
		t.Fatalf("ValidateHash returned error: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf(
			"expected hashes to match: generated %s, validated %s",
			hash1.String(),
			hash2.String(),
		)
	}

	// Tamper payload to trigger validation failure.
	s.sealedPayload = []byte{9, 9, 9}
	if _, err := s.ValidateHash(context.Background()); err == nil {
		t.Fatalf("expected validation error after tampering, got nil")
	}

	// Missing data for hash generation.
	invalid := SealedSlice{
		Nonce:         []byte{1},
		sealedPayload: []byte{1},
	}
	if _, err := invalid.GetHash(context.Background()); err == nil {
		t.Fatalf("expected error for missing fields, got nil")
	}
}

func TestSealedSliceHash_ComputesAndRemainsStable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	base := SealedSlice{
		ChunkHash:      hash.HashBytes([]byte("chunk")),
		RSDataSlices:   2,
		RSParitySlices: 1,
		RSSliceIndex:   1,
		Nonce:          []byte{1, 2, 3},
		sealedPayload:  []byte{4, 5, 6},
	}

	hash1, err := base.GetHash(ctx)
	if err != nil {
		t.Fatalf("GetHash returned error: %v", err)
	}
	if hash1 == (hash.Hash{}) {
		t.Fatalf("GetHash returned zero hash; expected non-zero")
	}

	hash2, err := base.GetHash(ctx)
	if err != nil {
		t.Fatalf("second GetHash returned error: %v", err)
	}
	if hash1 != hash2 {
		t.Fatalf(
			"hash changed without field modifications: first=%s second=%s",
			hash1.String(),
			hash2.String(),
		)
	}
}

func TestSealedSliceHash_ValidateFailsWhenHashRelevantFieldsChange(
	t *testing.T,
) {
	t.Parallel()

	ctx := context.Background()
	original := SealedSlice{
		ChunkHash:      hash.HashBytes([]byte("chunk")),
		RSDataSlices:   2,
		RSParitySlices: 1,
		RSSliceIndex:   1,
		Nonce:          []byte{1, 2, 3},
		sealedPayload:  []byte{4, 5, 6},
	}

	origHash, err := original.GetHash(ctx)
	if err != nil {
		t.Fatalf("GetHash returned error: %v", err)
	}

	makeCopy := func() SealedSlice {
		c := original
		c.Nonce = append([]byte(nil), original.Nonce...)
		c.sealedPayload = append([]byte(nil), original.sealedPayload...)
		return c
	}

	tests := []struct {
		name string
		mut  func(s *SealedSlice)
	}{
		{
			name: "ChunkHash",
			mut: func(s *SealedSlice) {
				s.ChunkHash = hash.HashBytes([]byte("other-chunk"))
			},
		},
		{
			name: "RSDataSlices",
			mut: func(s *SealedSlice) {
				s.RSDataSlices++
			},
		},
		{
			name: "RSParitySlices",
			mut: func(s *SealedSlice) {
				s.RSParitySlices++
			},
		},
		{
			name: "RSSliceIndex",
			mut: func(s *SealedSlice) {
				s.RSSliceIndex++
			},
		},
		{
			name: "Nonce",
			mut: func(s *SealedSlice) {
				s.Nonce = append([]byte(nil), s.Nonce...)
				s.Nonce[0] ^= 0xFF
			},
		},
		{
			name: "Payload",
			mut: func(s *SealedSlice) {
				s.sealedPayload = append([]byte(nil), s.sealedPayload...)
				s.sealedPayload[0] ^= 0xFF
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			modified := makeCopy()
			tt.mut(&modified)

			if _, err := modified.ValidateHash(ctx); err == nil {
				t.Fatalf(
					"ValidateHash succeeded after modifying %s; expected failure",
					tt.name,
				)
			}

			modified.Hash = hash.Hash{}
			newHash, err := modified.GetHash(ctx)
			if err != nil {
				t.Fatalf("GetHash returned error after modifying %s: %v", tt.name, err)
			}
			if newHash == origHash {
				t.Fatalf(
					"GetHash did not change after modifying %s: old=%s new=%s",
					tt.name,
					origHash.String(),
					newHash.String(),
				)
			}
		})
	}
}

func TestSealedSliceHash_NonHashFieldsDoNotAffectHash(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	s := SealedSlice{
		ChunkHash:      hash.HashBytes([]byte("chunk")),
		RSDataSlices:   2,
		RSParitySlices: 1,
		RSSliceIndex:   1,
		Nonce:          []byte{1, 2, 3},
		sealedPayload:  []byte{4, 5, 6},
	}

	h1, err := s.GetHash(ctx)
	if err != nil {
		t.Fatalf("GetHash returned error: %v", err)
	}

	s.cas = &CAS{}
	h2, err := s.ValidateHash(ctx)
	if err != nil {
		t.Fatalf(
			"ValidateHash returned error after changing non-hash field: %v",
			err,
		)
	}

	if h1 != h2 {
		t.Fatalf(
			"hash changed after modifying non-hash field: before=%s after=%s",
			h1.String(),
			h2.String(),
		)
	}
}

func TestValidateShardCount_BoundsChecks(t *testing.T) {
	t.Run("exceedsMaxAllowedForSliceConfig", func(t *testing.T) {
		err := validateShardCount(make([][]byte, 10), 2, 2)
		if err == nil {
			t.Fatalf("expected error when shard count exceeds total slices, got nil")
		}
	})

	t.Run("exceedsUint8Maximum", func(t *testing.T) {
		err := validateShardCount(make([][]byte, maxSliceCount+1), 200, 60)
		if err == nil {
			t.Fatalf("expected error when shard count exceeds uint8 maximum, got nil")
		}
	})
}

func TestChunk_GetContent_ErrorOnInsufficientSlices(t *testing.T) {
	payload := bytes.Repeat([]byte("insufficient-slices-"), 512)
	ctx, c, dr, chunk := storeBlobAndSelectChunk(t, payload)

	slices := dr.SealedSlicesByChunk()[chunk.Hash]
	if len(slices) == 0 {
		t.Fatalf(
			"test setup failure: no sealed slices stored for chunk %s",
			chunk.Hash.String(),
		)
	}
	dataSlices := int(slices[0].RSDataSlices)
	if len(slices) <= dataSlices {
		t.Fatalf(
			"test setup failure: expected > RSDataSlices slices, got %d (need > %d)",
			len(slices),
			dataSlices,
		)
	}

	// Remove slices so fewer than RSDataSlices remain.
	reduced := append([]SealedSlice(nil), slices[:dataSlices-1]...)
	if err := dr.ReplaceChunkSealedSlices(chunk.Hash, reduced); err != nil {
		t.Fatalf(
			"failed to replace sealed slices for chunk %s: %v",
			chunk.Hash.String(),
			err,
		)
	}

	_, err := chunk.GetContent(ctx, *c)
	if err == nil || !strings.Contains(err.Error(), "not enough slices") {
		t.Fatalf("expected reconstruction error due to missing slices, got %v", err)
	}
}

func TestChunk_GetContent_ErrorOnMixedRSParameters(t *testing.T) {
	payload := bytes.Repeat([]byte("mixed-rs-parameters-"), 512)
	ctx, c, dr, chunk := storeBlobAndSelectChunk(t, payload)

	slices := dr.SealedSlicesByChunk()[chunk.Hash]
	if len(slices) < 2 {
		t.Fatalf("test setup failure: need at least 2 slices, got %d", len(slices))
	}

	modified := append([]SealedSlice(nil), slices...)
	modified[1].RSParitySlices = modified[1].RSParitySlices + 1
	if err := dr.ReplaceChunkSealedSlices(chunk.Hash, modified); err != nil {
		t.Fatalf(
			"failed to replace sealed slices for chunk %s: %v",
			chunk.Hash.String(),
			err,
		)
	}

	_, err := chunk.GetContent(ctx, *c)
	if err == nil || !strings.Contains(err.Error(), "mixed RS parameters") {
		t.Fatalf("expected RS parameter mismatch error, got %v", err)
	}
}

func TestChunk_GetContent_ErrorOnCorruptedSlicePayload(t *testing.T) {
	payload := bytes.Repeat([]byte("corrupted-slice-payload-"), 512)
	ctx, c, dr, chunk := storeBlobAndSelectChunk(t, payload)

	slices := dr.SealedSlicesByChunk()[chunk.Hash]
	if len(slices) == 0 {
		t.Fatalf("test setup failure: no slices available for corruption test")
	}

	modified := append([]SealedSlice(nil), slices...)
	target := modified[0]

	payloadBytes, ok := dr.SealedPayloads()[target.Hash]
	if !ok || len(payloadBytes) == 0 {
		t.Fatalf(
			"test setup failure: no sealed payload for slice %s",
			target.Hash.String(),
		)
	}

	corrupted := append([]byte(nil), payloadBytes...)
	corrupted[0] ^= 0xFF
	target.sealedPayload = corrupted
	modified[0] = target
	if err := dr.ReplaceChunkSealedSlices(chunk.Hash, modified); err != nil {
		t.Fatalf(
			"failed to replace sealed slices for chunk %s: %v",
			chunk.Hash.String(),
			err,
		)
	}

	_, err := chunk.GetContent(ctx, *c)
	if err == nil ||
		!strings.Contains(err.Error(), "failed to decrypt sealed slice") {
		t.Fatalf("expected decryption failure for corrupted slice, got %v", err)
	}
}

func TestCAS_HashRegression_Anchor_12345678(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	payload := []byte("12345678")

	// Use a fixed timestamp and key so the blob hash is fully deterministic.
	const created int64 = 1730000000000 //  NEVER change it
	const key = "anchor/12345678"
	parent := hash.Hash{} // zero parent for the anchor

	// Build deterministic chunk size map and fake router/index.
	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	blob, err := cas.StoreBlob(ctx, payload, key, parent, created, *c)
	if err != nil {
		t.Fatalf("StoreBlob returned error for anchor payload: %v", err)
	}

	// Collect blob hash.
	gotBlobHash := blob.Hash.String()

	// Collect chunk hashes (sorted for determinism).
	chunksByHash := dr.ChunksByHash()
	gotChunkHashes := make([]string, 0, len(chunksByHash))
	for h := range chunksByHash {
		gotChunkHashes = append(gotChunkHashes, h.String())
	}
	sort.Strings(gotChunkHashes)

	// Collect sealed slice hashes (sorted), but we will not fix them as anchors
	// because they depend on encryption randomness.
	slicesByHash := dr.SealedSlicesByHash()
	gotSliceHashes := make([]string, 0, len(slicesByHash))
	for h := range slicesByHash {
		gotSliceHashes = append(gotSliceHashes, h.String())
	}
	sort.Strings(gotSliceHashes)

	// Log what we actually got so we can debug if the regression fails.
	t.Logf("ANCHOR blob hash = %s", gotBlobHash)
	t.Logf("ANCHOR chunk hashes = %v", gotChunkHashes)
	t.Logf("ANCHOR sealed slice hashes (NON-DETERMINISTIC) = %v", gotSliceHashes)

	// === REGRESSION ANCHOR VALUES (DETERMINISTIC ONLY) ===
	// These were observed for the current implementation. As long as the
	// implementation (chunking, hashing, blob metadata hashing) doesn't change,
	// these must stay the same.

	const expectedBlobHash = "" +
		"0000000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000000000000000000000000000000000000000000000000000"

	const expectedChunkHash0 = "" +
		"fa585d89c851dd338a70dcf535aa2a92fee7836dd6aff1226583e88e0996293f" +
		"16bc009c652826e0fc5c706695a03cddce372f139eff4d13959da6f1f5d3eabe"

	expectedChunkHashes := []string{
		expectedChunkHash0,
	}

	// We *do* expect a stable number of sealed slices for this input and RS
	// config.
	// This is deterministic as long as RS parameters and chunking stay the same.
	const expectedSealedSliceCount = 7

	// === ASSERTIONS ===

	if gotBlobHash != expectedBlobHash {
		t.Fatalf(
			"blob hash regression: expected %s, got %s",
			expectedBlobHash,
			gotBlobHash,
		)
	}

	if len(gotChunkHashes) != len(expectedChunkHashes) {
		t.Fatalf(
			"chunk hash count regression: expected %d, got %d (%v)",
			len(expectedChunkHashes),
			len(gotChunkHashes),
			gotChunkHashes,
		)
	}
	for i := range expectedChunkHashes {
		if gotChunkHashes[i] != expectedChunkHashes[i] {
			t.Fatalf(
				"chunk hash regression at index %d: expected %s, got %s",
				i,
				expectedChunkHashes[i],
				gotChunkHashes[i],
			)
		}
	}

	if len(gotSliceHashes) != expectedSealedSliceCount {
		t.Fatalf(
			"sealed slice count regression: expected %d, got %d (%v)",
			expectedSealedSliceCount,
			len(gotSliceHashes),
			gotSliceHashes,
		)
	}
}

func TestCAS_StoreBlob_RoundTrip_LargeBlob10MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 10MB roundtrip test in -short mode")
	}

	ctx := context.Background()
	const size = 10 * 1024 * 1024 // 10MB

	// Build a deterministic payload so any mismatch is reproducible.
	// Use a simple pattern to avoid big constants.
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()
	_, err := cas.StoreBlob(
		ctx,
		payload,
		"test/large-blob-key",
		hash.Hash{},
		created,
		*c,
	)
	if err != nil {
		t.Fatalf("StoreBlob returned error for 10MB blob: %v", err)
	}

	blob := firstStoredBlob(t, dr)

	content, err := blob.GetContent(ctx, *c)
	if err != nil {
		t.Fatalf("GetContent returned error for 10MB blob: %v", err)
	}

	if len(content) != len(payload) {
		t.Fatalf(
			"large blob length mismatch: expected %d bytes, got %d",
			len(payload),
			len(content),
		)
	}
	if !bytes.Equal(content, payload) {
		t.Fatalf("large blob content mismatch after roundtrip")
	}
}

func FuzzCAS_StoreBlob_RoundTrip2(f *testing.F) {
	ctx := context.Background()
	ki := newFakeKeyIndex()
	c := crypt.New()

	// Seed corpus
	f.Add([]byte("hello"))
	f.Add([]byte("12345678"))
	f.Add(bytes.Repeat([]byte("x"), 1024))

	f.Fuzz(func(t *testing.T, payload []byte) {
		if len(payload) == 0 {
			t.Skip("empty payload not interesting here")
		}

		chunkSizeMap := buildChunkSizeMap(t, payload)
		dr := newFakeDataRouter(chunkSizeMap)

		cas := NewCAS(dr, ki)
		created := time.Now().UnixMilli()

		_, err := cas.StoreBlob(
			ctx,
			payload,
			"fuzz/key",
			hash.Hash{},
			created,
			*c,
		)
		if err != nil {
			t.Fatalf("StoreBlob error: %v", err)
		}

		blob := firstStoredBlob(t, dr)

		got, err := blob.GetContent(ctx, *c)
		if err != nil {
			t.Fatalf("GetContent error: %v", err)
		}

		if !bytes.Equal(got, payload) {
			t.Fatalf(
				"roundtrip mismatch: len(got)=%d len(want)=%d",
				len(got),
				len(payload),
			)
		}
	})
}

func TestCAS_StoreBlob_SameContentSameChunkHashes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	payload := []byte("same-content-for-chunks")

	// First CAS instance
	chunkSizeMap1 := buildChunkSizeMap(t, payload)
	dr1 := newFakeDataRouter(chunkSizeMap1)
	ki1 := newFakeKeyIndex()
	c1 := crypt.New()
	cas1 := NewCAS(dr1, ki1)

	created1 := time.Now().UnixMilli()
	if _, err := cas1.StoreBlob(
		ctx,
		payload,
		"key/first",
		hash.Hash{}, // parent
		created1,
		*c1,
	); err != nil {
		t.Fatalf("first StoreBlob returned error: %v", err)
	}

	// Second CAS instance (different key / timestamp, same content)
	chunkSizeMap2 := buildChunkSizeMap(t, payload)
	dr2 := newFakeDataRouter(chunkSizeMap2)
	ki2 := newFakeKeyIndex()
	c2 := crypt.New()
	cas2 := NewCAS(dr2, ki2)

	created2 := created1 + 1234 // deliberately different timestamp
	if _, err := cas2.StoreBlob(
		ctx,
		payload,
		"key/second",
		hash.Hash{},
		created2,
		*c2,
	); err != nil {
		t.Fatalf("second StoreBlob returned error: %v", err)
	}

	// Collect chunk hashes from both fake data routers.
	chunksByHash1 := dr1.ChunksByHash()
	chunksByHash2 := dr2.ChunksByHash()

	if len(chunksByHash1) == 0 || len(chunksByHash2) == 0 {
		t.Fatalf(
			"expected chunks for payload, got len(chunks1)=%d len(chunks2)=%d",
			len(chunksByHash1),
			len(chunksByHash2),
		)
	}

	// Turn maps into sorted slices of hash strings for comparison.
	hashes1 := make([]string, 0, len(chunksByHash1))
	for h := range chunksByHash1 {
		hashes1 = append(hashes1, h.String())
	}
	sort.Strings(hashes1)

	hashes2 := make([]string, 0, len(chunksByHash2))
	for h := range chunksByHash2 {
		hashes2 = append(hashes2, h.String())
	}
	sort.Strings(hashes2)

	if len(hashes1) != len(hashes2) {
		t.Fatalf(
			"chunk hash count mismatch: first=%d second=%d\nfirst:  %v\nsecond: %v",
			len(hashes1),
			len(hashes2),
			hashes1,
			hashes2,
		)
	}

	for i := range hashes1 {
		if hashes1[i] != hashes2[i] {
			t.Fatalf(
				"chunk hash mismatch at index %d: first=%s second=%s",
				i,
				hashes1[i],
				hashes2[i],
			)
		}
	}
}

func TestCAS_StoreBlob_RoundTrip_LargeBlob5MB(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 5MB roundtrip test in -short mode")
	}

	ctx := context.Background()
	const size = 5 * 1024 * 1024 // 5MB

	// Build a deterministic payload: simple repeating pattern.
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()
	_, err := cas.StoreBlob(
		ctx,
		payload,
		"test/roundtrip-5mb",
		hash.Hash{},
		created,
		*c,
	)
	if err != nil {
		t.Fatalf("StoreBlob returned error for 5MB blob: %v", err)
	}

	blob := firstStoredBlob(t, dr)

	got, err := blob.GetContent(ctx, *c)
	if err != nil {
		t.Fatalf("GetContent returned error for 5MB blob: %v", err)
	}

	if len(got) != len(payload) {
		t.Fatalf(
			"5MB blob length mismatch: expected %d bytes, got %d",
			len(payload),
			len(got),
		)
	}

	if !bytes.Equal(got, payload) {
		t.Fatalf("5MB blob content mismatch after roundtrip")
	}
}

func TestBlob_getChunkHashes_LoadsFromDataRouterAndCaches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	payload := []byte("blob-get-chunk-hashes-test")

	chunkSizeMap := buildChunkSizeMap(t, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()
	_, err := cas.StoreBlob(
		ctx,
		payload,
		"test/blob-get-chunk-hashes",
		hash.Hash{},
		created,
		*c,
	)
	if err != nil {
		t.Fatalf("StoreBlob returned error: %v", err)
	}

	// Get the stored blob as seen by the data router.
	original := firstStoredBlob(t, dr)

	if len(original.chunks) == 0 {
		t.Fatalf(
			"expected original blob to have chunk hashes, got 0",
		)
	}

	// Create a copy with chunks cleared, to force lazy load via cas.dr.
	blob := original
	blob.chunks = nil

	if len(blob.chunks) != 0 {
		t.Fatalf(
			"expected test blob to start without chunks, got %d",
			len(blob.chunks),
		)
	}

	// Call the method under test.
	chunks, err := blob.GetChunkHashes(ctx)
	if err != nil {
		t.Fatalf("GetChunkHashes returned error: %v", err)
	}

	if len(chunks) != len(original.chunks) {
		t.Fatalf(
			"expected %d chunks, got %d",
			len(original.chunks),
			len(chunks),
		)
	}

	for i := range chunks {
		if chunks[i] != original.chunks[i] {
			t.Fatalf(
				"chunk hash mismatch at index %d: want %s, got %s",
				i,
				original.chunks[i].String(),
				chunks[i].String(),
			)
		}
	}

	// And it should have populated the cache on the blob itself.
	if len(blob.chunks) != len(chunks) {
		t.Fatalf(
			"expected blob.chunks to be cached with %d hashes, got %d",
			len(chunks),
			len(blob.chunks),
		)
	}
}

func TestCAS_GetBlob_DelegatesToDataRouter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		t.Helper()

		dr := newFakeDataRouter(nil)
		ki := newFakeKeyIndex()
		cas := NewCAS(dr, ki)

		h := hash.HashBytes([]byte("blob-ok"))
		blob := Blob{
			Hash:    h,
			Key:     "key-ok",
			Parent:  hash.Hash{},
			Created: time.Now().UnixMilli(),
		}

		// Seed fake router.
		dr.blobs[h] = blob

		got, err := cas.GetBlob(ctx, h)
		if err != nil {
			t.Fatalf("GetBlob returned error: %v", err)
		}
		if got.Hash != h {
			t.Fatalf(
				"GetBlob returned blob with wrong hash: want %s, got %s",
				h.String(),
				got.Hash.String(),
			)
		}
	})

	t.Run("not-found", func(t *testing.T) {
		t.Helper()

		dr := newFakeDataRouter(nil)
		ki := newFakeKeyIndex()
		cas := NewCAS(dr, ki)

		h := hash.HashBytes([]byte("blob-missing"))

		_, err := cas.GetBlob(ctx, h)
		if err == nil {
			t.Fatalf(
				"expected error for missing blob %s, got nil",
				h.String(),
			)
		}
	})
}

func TestValidateReconstructionParameters_Valid(t *testing.T) {
	chunkHash := hash.HashBytes([]byte("chunk"))

	first := SealedSlice{
		ChunkHash:      chunkHash,
		RSDataSlices:   2,
		RSParitySlices: 1,
	}

	params, err := validateReconstructionParameters(first)
	if err != nil {
		t.Fatalf(
			"expected valid reconstruction params, got error: %v",
			err,
		)
	}

	if params.k != 2 {
		t.Fatalf("expected k=2, got %d", params.k)
	}
	if params.p != 1 {
		t.Fatalf("expected p=1, got %d", params.p)
	}
	if params.total != 3 {
		t.Fatalf("expected total=3, got %d", params.total)
	}
	if params.chunkHash != chunkHash {
		t.Fatalf(
			"expected chunkHash=%s, got %s",
			chunkHash.String(),
			params.chunkHash.String(),
		)
	}
}

func TestValidateReconstructionParameters_InvalidRSDataSlices(t *testing.T) {
	first := SealedSlice{
		ChunkHash:      hash.HashBytes([]byte("chunk-invalid-k")),
		RSDataSlices:   0, // invalid
		RSParitySlices: 1,
	}

	_, err := validateReconstructionParameters(first)
	if err == nil {
		t.Fatalf("expected error for RSDataSlices=0, got nil")
	}
	if !strings.Contains(err.Error(), "invalid RSDataSlices") {
		t.Fatalf(
			"expected error about invalid RSDataSlices, got: %v",
			err,
		)
	}
}

func BenchmarkCAS_StoreBlob_1MiB(b *testing.B) {
	ctx := context.Background()
	const size = 1 * 1024 * 1024 // 1 MiB

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	chunkSizeMap := buildChunkSizeMap(b, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	b.ResetTimer()
	b.ReportAllocs()

	iter := 0
	for b.Loop() {
		key := fmt.Sprintf("bench/store-1mib/%d", iter)
		created := time.Now().UnixMilli()

		_, err := cas.StoreBlob(
			ctx,
			payload,
			key,
			hash.Hash{},
			created,
			*c,
		)
		if err != nil {
			b.Fatalf("StoreBlob failed at iter %d: %v", iter, err)
		}
		iter++
	}
}

func BenchmarkCAS_StoreAndGet_RoundTrip_5MiB(b *testing.B) {
	ctx := context.Background()
	const size = 5 * 1024 * 1024 // 5 MiB

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	chunkSizeMap := buildChunkSizeMap(b, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	b.ResetTimer()
	b.ReportAllocs()

	iter := 0
	for b.Loop() {
		key := fmt.Sprintf("bench/roundtrip-5mib/%d", iter)
		created := time.Now().UnixMilli()

		_, err := cas.StoreBlob(
			ctx,
			payload,
			key,
			hash.Hash{},
			created,
			*c,
		)
		if err != nil {
			b.Fatalf("StoreBlob failed at iter %d: %v", iter, err)
		}

		blob := firstStoredBlob(b, dr)

		got, err := blob.GetContent(ctx, *c)
		if err != nil {
			b.Fatalf("GetContent failed at iter %d: %v", iter, err)
		}
		if len(got) != len(payload) {
			b.Fatalf(
				"len mismatch at iter %d: want %d, got %d",
				iter,
				len(payload),
				len(got),
			)
		}
		iter++
	}
}

func BenchmarkBlob_GetContent_5MiB(b *testing.B) {
	ctx := context.Background()
	const size = 5 * 1024 * 1024 // 5 MiB

	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	chunkSizeMap := buildChunkSizeMap(b, payload)
	dr := newFakeDataRouter(chunkSizeMap)
	ki := newFakeKeyIndex()
	c := crypt.New()
	cas := NewCAS(dr, ki)

	created := time.Now().UnixMilli()

	// Store once to populate the fake router.
	_, err := cas.StoreBlob(
		ctx,
		payload,
		"bench/getContent-5mib",
		hash.Hash{},
		created,
		*c,
	)
	if err != nil {
		b.Fatalf("StoreBlob failed in setup: %v", err)
	}

	// Use the stored blob from the fake data router, as in other tests.
	blob := firstStoredBlob(b, dr)

	// Optional warm-up to populate any lazy caches.
	if _, err := blob.GetContent(ctx, *c); err != nil {
		b.Fatalf("GetContent warm-up failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		got, err := blob.GetContent(ctx, *c)
		if err != nil {
			b.Fatalf("GetContent failed during loop: %v", err)
		}
		if len(got) != len(payload) {
			b.Fatalf(
				"len mismatch during loop: want %d, got %d",
				len(payload),
				len(got),
			)
		}
	}
}

func BenchmarkValidateReconstructionParameters(b *testing.B) {
	s := SealedSlice{
		ChunkHash:      hash.HashBytes([]byte("bench-chunk")),
		RSDataSlices:   10,
		RSParitySlices: 4,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		params, err := validateReconstructionParameters(s)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
		if params.k != 10 || params.p != 4 || params.total != 14 {
			b.Fatalf(
				"unexpected params: k=%d p=%d total=%d",
				params.k,
				params.p,
				params.total,
			)
		}
	}
}
