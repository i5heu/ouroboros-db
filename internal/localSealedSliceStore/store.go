package localSealedSliceStore

import (
	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cas"
)

// LocalSealedSliceStore manages the local persistence of SealedSlices.
// It uses BadgerDB as the underlying storage engine.
type LocalSealedSliceStore interface {
	// Store persists a SealedSlice associated with a public key hash.
	// Returns the hash of the stored slice or an error.
	Store(
		sealedSlice cas.SealedSliceWithPayload,
		hashOfPubKey hash.Hash,
	) (hash.Hash, error)

	// Get retrieves a SealedSlice by its hash.
	Get(sliceHash hash.Hash) (cas.SealedSlice, error)

	// Delete removes a SealedSlice by its hash.
	Delete(sliceHash hash.Hash) error

	// ChunkListSealedSlices returns all SealedSlice hashes for a given Chunk
	// hash.
	ChunkListSealedSlices(chunkHash hash.Hash) ([]hash.Hash, error)

	// ChunkListSealedSlicesForPubKey returns SealedSlice hashes for a given Chunk
	// hash and Public Key hash.
	ChunkListSealedSlicesForPubKey(
		chunkHash, hashOfPubKey hash.Hash,
	) ([]hash.Hash, error)
}

// Store implements LocalSealedSliceStore using BadgerDB.
type Store struct {
	db *badger.DB
}

// New creates a new Store instance.
func New(db *badger.DB) *Store { // A
	return &Store{
		db: db,
	}
}

// Store persists a SealedSlice associated with a public key hash.
// Key format: [ChunkHash]:[HashOfPubkey]:[SealedSliceHash] (binary)
func (s *Store) Store(
	sealedSlice cas.SealedSlice,
	hashOfPubKey hash.Hash,
) (hash.Hash, error) { // A
	// TODO: Implement storage logic
	var zeroHash hash.Hash
	return zeroHash, nil
}

// Get retrieves a SealedSlice by its hash.
func (s *Store) Get(sliceHash hash.Hash) (cas.SealedSlice, error) { // A
	// TODO: Implement retrieval logic
	return cas.SealedSlice{}, nil
}

// Delete removes a SealedSlice by its hash.
func (s *Store) Delete(sliceHash hash.Hash) error { // A
	// TODO: Implement deletion logic
	return nil
}

// ChunkListSealedSlices returns all SealedSlice hashes for a given Chunk hash.
func (s *Store) ChunkListSealedSlices(
	chunkHash hash.Hash,
) ([]hash.Hash, error) { // A
	// TODO: Implement listing logic
	return nil, nil
}

// ChunkListSealedSlicesForPubKey returns SealedSlice hashes for a given Chunk
// hash and Public Key hash.
func (s *Store) ChunkListSealedSlicesForPubKey(
	chunkHash, hashOfPubKey hash.Hash,
) ([]hash.Hash, error) { // A
	// TODO: Implement listing logic
	return nil, nil
}
