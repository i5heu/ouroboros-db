package localSealedSliceStore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"reflect"
	"unsafe"

	"github.com/dgraph-io/badger/v4"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/cas"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
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

const (
	keyDelimiter = byte(':')
	metaSuffix   = ":meta"
)

var hashLength = len(hash.Hash{})

// Store persists a SealedSlice associated with a public key hash.
// Key format: [ChunkHash]:[HashOfPubkey]:[SealedSliceHash] (binary)
func (s *Store) Store(
	sealedSlice cas.SealedSliceWithPayload,
	hashOfPubKey hash.Hash,
) (hash.Hash, error) { // A
	if s.db == nil {
		return hash.Hash{}, errors.New("localSealedSliceStore: db is nil")
	}

	if sealedSlice.ChunkHash == (hash.Hash{}) {
		return hash.Hash{}, errors.New("localSealedSliceStore: chunk hash is required")
	}
	if hashOfPubKey == (hash.Hash{}) {
		return hash.Hash{}, errors.New("localSealedSliceStore: pub key hash is required")
	}
	if len(sealedSlice.SealedPayload) == 0 {
		return hash.Hash{}, errors.New("localSealedSliceStore: sealed payload is required")
	}
	if len(sealedSlice.Nonce) == 0 {
		return hash.Hash{}, errors.New("localSealedSliceStore: nonce is required")
	}
	if sealedSlice.RSDataSlices == 0 || sealedSlice.RSParitySlices == 0 {
		return hash.Hash{}, errors.New(
			"localSealedSliceStore: RSDataSlices and RSParitySlices must be > 0",
		)
	}

	sliceHash, err := ensureSealedSliceHash(sealedSlice)
	if err != nil {
		return hash.Hash{}, err
	}

	baseKey := buildBaseKey(sealedSlice.ChunkHash, hashOfPubKey, sliceHash)

	metaBytes, err := encodeMetadata(sealedSlice, hashOfPubKey, sliceHash)
	if err != nil {
		return hash.Hash{}, err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(baseKey, sealedSlice.SealedPayload); err != nil {
			return fmt.Errorf("localSealedSliceStore: store payload: %w", err)
		}
		if err := txn.Set(metaKey(baseKey), metaBytes); err != nil {
			return fmt.Errorf("localSealedSliceStore: store metadata: %w", err)
		}
		return nil
	})
	if err != nil {
		return hash.Hash{}, err
	}

	return sliceHash, nil
}

// Get retrieves a SealedSlice by its hash.
func (s *Store) Get(sliceHash hash.Hash) (cas.SealedSlice, error) { // A
	if s.db == nil {
		return cas.SealedSlice{}, errors.New("localSealedSliceStore: db is nil")
	}

	baseKey, err := s.findBaseKeyBySliceHash(sliceHash)
	if err != nil {
		return cas.SealedSlice{}, err
	}

	var sealed cas.SealedSlice
	err = s.db.View(func(txn *badger.Txn) error {
		payloadItem, err := txn.Get(baseKey)
		if err != nil {
			return fmt.Errorf("localSealedSliceStore: payload not found: %w", err)
		}
		payload, err := payloadItem.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("localSealedSliceStore: copy payload: %w", err)
		}

		metaItem, err := txn.Get(metaKey(baseKey))
		if err != nil {
			return fmt.Errorf("localSealedSliceStore: metadata not found: %w", err)
		}
		metaBytes, err := metaItem.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("localSealedSliceStore: copy metadata: %w", err)
		}

		meta, err := decodeMetadata(metaBytes)
		if err != nil {
			return err
		}

		if meta.SealedSliceHash != sliceHash {
			return fmt.Errorf(
				"localSealedSliceStore: metadata hash mismatch: expected %s got %s",
				sliceHash.String(),
				meta.SealedSliceHash.String(),
			)
		}

		sealed = cas.SealedSlice{
			Hash:           meta.SealedSliceHash,
			ChunkHash:      meta.ChunkHash,
			RSDataSlices:   meta.RSDataSlices,
			RSParitySlices: meta.RSParitySlices,
			RSSliceIndex:   meta.RSSliceIndex,
			Nonce:          meta.Nonce,
		}
		setSealedPayload(&sealed, payload)
		return nil
	})
	if err != nil {
		return cas.SealedSlice{}, err
	}

	return sealed, nil
}

// Delete removes a SealedSlice by its hash.
func (s *Store) Delete(sliceHash hash.Hash) error { // A
	if s.db == nil {
		return errors.New("localSealedSliceStore: db is nil")
	}

	baseKey, err := s.findBaseKeyBySliceHash(sliceHash)
	if err != nil {
		return err
	}

	err = s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete(baseKey); err != nil {
			return fmt.Errorf("localSealedSliceStore: delete payload: %w", err)
		}
		if err := txn.Delete(metaKey(baseKey)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("localSealedSliceStore: delete metadata: %w", err)
		}
		return nil
	})
	return err
}

// ChunkListSealedSlices returns all SealedSlice hashes for a given Chunk hash.
func (s *Store) ChunkListSealedSlices(
	chunkHash hash.Hash,
) ([]hash.Hash, error) { // A
	if s.db == nil {
		return nil, errors.New("localSealedSliceStore: db is nil")
	}

	if chunkHash == (hash.Hash{}) {
		return nil, errors.New("localSealedSliceStore: chunk hash is required")
	}

	prefix := chunkPrefix(chunkHash)
	var hashes []hash.Hash

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			if isMetaKey(key) {
				continue
			}
			_, _, sliceHash, err := parseBaseKey(key)
			if err != nil {
				return err
			}
			hashes = append(hashes, sliceHash)
		}
		return nil
	})

	return hashes, err
}

// ChunkListSealedSlicesForPubKey returns SealedSlice hashes for a given Chunk
// hash and Public Key hash.
func (s *Store) ChunkListSealedSlicesForPubKey(
	chunkHash, hashOfPubKey hash.Hash,
) ([]hash.Hash, error) { // A
	if s.db == nil {
		return nil, errors.New("localSealedSliceStore: db is nil")
	}

	if chunkHash == (hash.Hash{}) {
		return nil, errors.New("localSealedSliceStore: chunk hash is required")
	}
	if hashOfPubKey == (hash.Hash{}) {
		return nil, errors.New("localSealedSliceStore: pub key hash is required")
	}

	prefix := chunkPubPrefix(chunkHash, hashOfPubKey)
	var hashes []hash.Hash

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = prefix
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			if isMetaKey(key) {
				continue
			}
			_, _, sliceHash, err := parseBaseKey(key)
			if err != nil {
				return err
			}
			hashes = append(hashes, sliceHash)
		}
		return nil
	})

	return hashes, err
}

func ensureSealedSliceHash(
	sealedSlice cas.SealedSliceWithPayload,
) (hash.Hash, error) {
	if sealedSlice.Hash != (hash.Hash{}) {
		computed := computeSliceHash(
			sealedSlice.ChunkHash,
			sealedSlice.RSDataSlices,
			sealedSlice.RSParitySlices,
			sealedSlice.RSSliceIndex,
			sealedSlice.Nonce,
			sealedSlice.SealedPayload,
		)
		if computed != sealedSlice.Hash {
			return hash.Hash{}, fmt.Errorf(
				"localSealedSliceStore: provided hash mismatch: expected %s got %s",
				computed.String(),
				sealedSlice.Hash.String(),
			)
		}
		return sealedSlice.Hash, nil
	}

	return computeSliceHash(
		sealedSlice.ChunkHash,
		sealedSlice.RSDataSlices,
		sealedSlice.RSParitySlices,
		sealedSlice.RSSliceIndex,
		sealedSlice.Nonce,
		sealedSlice.SealedPayload,
	), nil
}

func computeSliceHash(
	chunkHash hash.Hash,
	rsData, rsParity, rsIndex uint8,
	nonce, payload []byte,
) hash.Hash {
	buf := make(
		[]byte,
		0,
		hashLength+3+len(nonce)+len(payload),
	)
	buf = append(buf, chunkHash[:]...)
	buf = append(buf, rsData, rsParity, rsIndex)
	buf = append(buf, nonce...)
	buf = append(buf, payload...)
	return hash.HashBytes(buf)
}

func buildBaseKey(
	chunkHash, pubKeyHash, sliceHash hash.Hash,
) []byte {
	key := make([]byte, hashLength*3+2)

	copy(key[0:hashLength], chunkHash[:])
	key[hashLength] = keyDelimiter
	copy(key[hashLength+1:hashLength+1+hashLength], pubKeyHash[:])
	key[hashLength*2+1] = keyDelimiter
	copy(key[hashLength*2+2:], sliceHash[:])

	return key
}

func metaKey(baseKey []byte) []byte {
	out := make([]byte, len(baseKey)+len(metaSuffix))
	copy(out, baseKey)
	copy(out[len(baseKey):], []byte(metaSuffix))
	return out
}

func chunkPrefix(chunkHash hash.Hash) []byte {
	prefix := make([]byte, hashLength+1)
	copy(prefix, chunkHash[:])
	prefix[hashLength] = keyDelimiter
	return prefix
}

func chunkPubPrefix(chunkHash, pubKeyHash hash.Hash) []byte {
	prefix := make([]byte, hashLength*2+2)
	copy(prefix, chunkHash[:])
	prefix[hashLength] = keyDelimiter
	copy(prefix[hashLength+1:], pubKeyHash[:])
	prefix[len(prefix)-1] = keyDelimiter
	return prefix
}

func isMetaKey(key []byte) bool {
	return bytes.HasSuffix(key, []byte(metaSuffix))
}

func parseBaseKey(
	key []byte,
) (hash.Hash, hash.Hash, hash.Hash, error) {
	expectedLen := hashLength*3 + 2
	if len(key) != expectedLen {
		return hash.Hash{}, hash.Hash{}, hash.Hash{},
			fmt.Errorf("localSealedSliceStore: invalid key length %d", len(key))
	}

	if key[hashLength] != keyDelimiter || key[hashLength*2+1] != keyDelimiter {
		return hash.Hash{}, hash.Hash{}, hash.Hash{},
			fmt.Errorf("localSealedSliceStore: invalid key delimiter")
	}

	var chunkHash hash.Hash
	var pubKeyHash hash.Hash
	var sliceHash hash.Hash

	copy(chunkHash[:], key[0:hashLength])
	copy(pubKeyHash[:], key[hashLength+1:hashLength*2+1])
	copy(sliceHash[:], key[hashLength*2+2:])

	return chunkHash, pubKeyHash, sliceHash, nil
}

type sliceMetadata struct {
	ChunkHash       hash.Hash
	PubKeyHash      hash.Hash
	SealedSliceHash hash.Hash
	RSDataSlices    uint8
	RSParitySlices  uint8
	RSSliceIndex    uint8
	Nonce           []byte
}

func encodeMetadata(
	sealedSlice cas.SealedSliceWithPayload,
	pubKeyHash, sliceHash hash.Hash,
) ([]byte, error) {
	metaStruct, err := structpb.NewStruct(map[string]interface{}{
		"chunkHash":       sealedSlice.ChunkHash.String(),
		"pubKeyHash":      pubKeyHash.String(),
		"sealedSliceHash": sliceHash.String(),
		"rsDataSlices":    int64(sealedSlice.RSDataSlices),
		"rsParitySlices":  int64(sealedSlice.RSParitySlices),
		"rsSliceIndex":    int64(sealedSlice.RSSliceIndex),
		"nonce":           base64.StdEncoding.EncodeToString(sealedSlice.Nonce),
	})
	if err != nil {
		return nil, fmt.Errorf("localSealedSliceStore: build metadata struct: %w", err)
	}

	metaBytes, err := proto.Marshal(metaStruct)
	if err != nil {
		return nil, fmt.Errorf("localSealedSliceStore: marshal metadata: %w", err)
	}

	return metaBytes, nil
}

func decodeMetadata(metaBytes []byte) (sliceMetadata, error) {
	var metaStruct structpb.Struct
	if err := proto.Unmarshal(metaBytes, &metaStruct); err != nil {
		return sliceMetadata{}, fmt.Errorf("localSealedSliceStore: unmarshal metadata: %w", err)
	}

	fields := metaStruct.AsMap()

	chunkHash, err := parseHashField(fields, "chunkHash")
	if err != nil {
		return sliceMetadata{}, err
	}
	pubKeyHash, err := parseHashField(fields, "pubKeyHash")
	if err != nil {
		return sliceMetadata{}, err
	}
	sliceHash, err := parseHashField(fields, "sealedSliceHash")
	if err != nil {
		return sliceMetadata{}, err
	}
	rsData, err := parseUint8Field(fields, "rsDataSlices")
	if err != nil {
		return sliceMetadata{}, err
	}
	rsParity, err := parseUint8Field(fields, "rsParitySlices")
	if err != nil {
		return sliceMetadata{}, err
	}
	rsIndex, err := parseUint8Field(fields, "rsSliceIndex")
	if err != nil {
		return sliceMetadata{}, err
	}
	nonceStr, err := parseStringField(fields, "nonce")
	if err != nil {
		return sliceMetadata{}, err
	}
	nonce, err := base64.StdEncoding.DecodeString(nonceStr)
	if err != nil {
		return sliceMetadata{}, fmt.Errorf("localSealedSliceStore: decode nonce: %w", err)
	}

	return sliceMetadata{
		ChunkHash:       chunkHash,
		PubKeyHash:      pubKeyHash,
		SealedSliceHash: sliceHash,
		RSDataSlices:    rsData,
		RSParitySlices:  rsParity,
		RSSliceIndex:    rsIndex,
		Nonce:           nonce,
	}, nil
}

func parseHashField(fields map[string]interface{}, key string) (hash.Hash, error) {
	value, err := parseStringField(fields, key)
	if err != nil {
		return hash.Hash{}, err
	}

	parsed, err := hash.HashHexadecimal(value)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("localSealedSliceStore: parse %s: %w", key, err)
	}
	return parsed, nil
}

func parseStringField(fields map[string]interface{}, key string) (string, error) {
	raw, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("localSealedSliceStore: metadata missing %s", key)
	}
	str, ok := raw.(string)
	if ok {
		return str, nil
	}
	return "", fmt.Errorf("localSealedSliceStore: metadata %s not a string", key)
}

func parseUint8Field(fields map[string]interface{}, key string) (uint8, error) {
	raw, ok := fields[key]
	if !ok {
		return 0, fmt.Errorf("localSealedSliceStore: metadata missing %s", key)
	}

	switch v := raw.(type) {
	case float64:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("localSealedSliceStore: metadata %s out of range", key)
		}
		return uint8(v), nil
	case int64:
		if v < 0 || v > 255 {
			return 0, fmt.Errorf("localSealedSliceStore: metadata %s out of range", key)
		}
		return uint8(v), nil
	default:
		return 0, fmt.Errorf("localSealedSliceStore: metadata %s not numeric", key)
	}
}

func setSealedPayload(slice *cas.SealedSlice, payload []byte) {
	value := reflect.ValueOf(slice).Elem().FieldByName("sealedPayload")
	if !value.IsValid() {
		return
	}
	reflect.NewAt(value.Type(), unsafe.Pointer(value.UnsafeAddr())).
		Elem().
		SetBytes(payload)
}

func (s *Store) findBaseKeyBySliceHash(sliceHash hash.Hash) ([]byte, error) {
	var foundKey []byte

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			if isMetaKey(key) {
				continue
			}

			_, _, parsedSliceHash, err := parseBaseKey(key)
			if err != nil {
				return err
			}
			if parsedSliceHash == sliceHash {
				foundKey = append([]byte(nil), key...)
				return nil
			}
		}
		return errors.New("localSealedSliceStore: sealed slice not found")
	})

	return foundKey, err
}
