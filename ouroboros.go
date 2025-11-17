/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/hash"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

// OuroborosDB is the main database handle. It owns the crypt layer, the KV
// store, and the lifecycle of background components.
type OuroborosDB struct {
	log    *slog.Logger
	config Config

	crypt *crypt.Crypt
	kv    atomic.Pointer[ouroboroskv.KV]

	started   atomic.Bool
	startOnce sync.Once
	closeOnce sync.Once
}

var (
	ErrNotStarted  = errors.New("ouroboros: database not started")
	ErrClosed      = errors.New("ouroboros: database closed")
	ErrInvalidData = errors.New("ouroboros: invalid stored data")
)

// Config configures the database instance. Only Paths[0] is used at the
// moment; future versions may use multiple paths for sharding or tiering.
type Config struct {
	// Paths contains data directories. Currently only Paths[0] is used.
	Paths []string
	// MinimumFreeGB is a free-space threshold for on-disk operations.
	MinimumFreeGB uint
	// Logger is an optional structured logger. If nil, a stderr logger is used.
	Logger *slog.Logger
}

// defaultLogger returns a logger that writes text logs to stderr at Info level.
// Applications can inject their own slog.Logger for JSON, different levels, etc.
func defaultLogger() *slog.Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}

// New constructs a database handle. New does not perform heavy I/O or start
// background goroutines. Call Start to initialize subsystems.
func New(conf Config) (*OuroborosDB, error) {
	if len(conf.Paths) == 0 {
		return nil, fmt.Errorf("at least one path must be provided in config")
	}
	if conf.Logger == nil {
		conf.Logger = defaultLogger()
	}
	return &OuroborosDB{
		log:    conf.Logger,
		config: conf,
	}, nil
}

// Start initializes the crypt layer and KV store and marks the database as
// ready. Start is safe to call multiple times; only the first call has effect.
func (ou *OuroborosDB) Start(ctx context.Context) error {
	var startErr error
	ou.startOnce.Do(func() {
		dataRoot := ou.config.Paths[0]
		if err := os.MkdirAll(dataRoot, 0o700); err != nil {
			startErr = fmt.Errorf("mkdir %s: %w", dataRoot, err)
			return
		}

		// Initialize ouroboros-crypt from Paths[0]/ouroboros.key.
		cryptKeyPath := filepath.Join(dataRoot, "ouroboros.key")
		c, err := crypt.NewFromFile(cryptKeyPath)
		if err != nil {
			startErr = fmt.Errorf("init crypt: %w", err)
			return
		}

		// Initialize KV under Paths[0]/kv.
		kvConfig := &ouroboroskv.Config{
			Paths:            []string{filepath.Join(dataRoot, "kv")},
			MinimumFreeSpace: int(ou.config.MinimumFreeGB),
			Logger:           ou.log, // assumes ouroboros-kv also uses slog.Logger
		}
		kv, err := ouroboroskv.Init(c, kvConfig)
		if err != nil {
			startErr = fmt.Errorf("init kv: %w", err)
			return
		}

		ou.crypt = c
		ou.kv.Store(kv)

		ou.started.Store(true)
		ou.log.Info("OuroborosDB started", "path", dataRoot)
	})
	return startErr
}

// Run starts the database, then blocks until ctx is canceled, and finally
// performs a bounded graceful shutdown. It is a convenience for services.
func (ou *OuroborosDB) Run(ctx context.Context) error {
	if err := ou.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ou.Close(shutdownCtx)
}

// Close terminates background components and releases resources. Close is
// idempotent and safe to call multiple times.
func (ou *OuroborosDB) Close(ctx context.Context) error {
	var closeErr error
	ou.closeOnce.Do(func() {
		if kv := ou.kv.Swap(nil); kv != nil {
			if err := kv.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("close kv: %w", err))
			}
		}
		// Add crypt teardown here if the crypt package provides one.

		ou.log.Info("OuroborosDB closed")
	})
	return closeErr
}

// CloseWithoutContext closes the database using a background context.
// Prefer Close(ctx) to enforce an application-specific shutdown deadline.
func (ou *OuroborosDB) CloseWithoutContext() error {
	return ou.Close(context.Background())
}

func (ou *OuroborosDB) kvHandle() (*ouroboroskv.KV, error) {
	if !ou.started.Load() {
		return nil, ErrNotStarted
	}

	kv := ou.kv.Load()
	if kv == nil {
		return nil, ErrClosed
	}

	return kv, nil
}

func mergeHashSets(groups ...[]hash.Hash) []hash.Hash {
	if len(groups) == 0 {
		return nil
	}

	seen := make(map[hash.Hash]struct{})
	for _, list := range groups {
		for _, h := range list {
			if h.IsZero() {
				continue
			}
			seen[h] = struct{}{}
		}
	}

	if len(seen) == 0 {
		return nil
	}

	merged := make([]hash.Hash, 0, len(seen))
	for h := range seen {
		merged = append(merged, h)
	}

	sort.Slice(merged, func(i, j int) bool {
		return bytes.Compare(merged[i][:], merged[j][:]) < 0
	})

	return merged
}

const (
	payloadHeaderSize           = 256
	payloadHeaderNotExisting    = 0x00
	payloadHeaderExisting       = 0x10
	payloadHeaderIsMime         = 0x20
	payloadHeaderContentSizeLen = payloadHeaderSize - 1
)

type StoreOptions struct {
	Parent                  hash.Hash
	Children                []hash.Hash
	ReedSolomonShards       uint8
	ReedSolomonParityShards uint8
	MimeType                string
}

func (opts *StoreOptions) applyDefaults() {
	if opts.ReedSolomonShards == 0 {
		opts.ReedSolomonShards = 4
	}
	if opts.ReedSolomonParityShards == 0 {
		opts.ReedSolomonParityShards = 2
	}
}

func (ou *OuroborosDB) StoreData(ctx context.Context, content []byte, opts StoreOptions) (hash.Hash, error) {
	if err := ctx.Err(); err != nil {
		return hash.Hash{}, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return hash.Hash{}, err
	}

	opts.applyDefaults()

	encodedContent, err := encodeContentWithMimeType(content, opts.MimeType)
	if err != nil {
		return hash.Hash{}, err
	}

	metaBytes, err := encodeMetadata(storedMetadata{CreatedAt: time.Now().UTC()})
	if err != nil {
		return hash.Hash{}, fmt.Errorf("encode metadata: %w", err)
	}

	data := ouroboroskv.Data{
		MetaData:       metaBytes,
		Content:        encodedContent,
		RSDataSlices:   opts.ReedSolomonShards,
		RSParitySlices: opts.ReedSolomonParityShards,
	}

	if !opts.Parent.IsZero() {
		data.Parent = opts.Parent
	}
	for _, child := range opts.Children {
		if !child.IsZero() {
			data.Children = append(data.Children, child)
		}
	}

	dataHash, err := kv.WriteData(data)
	if err != nil {
		return hash.Hash{}, err
	}

	return dataHash, nil
}

func (ou *OuroborosDB) ListData(ctx context.Context) ([]hash.Hash, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return nil, err
	}

	keys, err := kv.ListRootKeys()
	if err != nil {
		return nil, err
	}

	return keys, nil
}

func (ou *OuroborosDB) ListChildren(ctx context.Context, parent hash.Hash) ([]hash.Hash, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return nil, err
	}

	return kv.GetChildren(parent)
}

type RetrievedData struct {
	Key       hash.Hash
	Content   []byte
	MimeType  string
	IsText    bool
	Parent    hash.Hash
	Children  []hash.Hash
	CreatedAt time.Time
}

func (ou *OuroborosDB) GetData(ctx context.Context, key hash.Hash) (RetrievedData, error) {
	if err := ctx.Err(); err != nil {
		return RetrievedData{}, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return RetrievedData{}, err
	}

	data, err := kv.ReadData(key)
	if err != nil {
		return RetrievedData{}, err
	}

	content, payloadHeader, isMime, err := ou.decodeContent(data.Content)
	if err != nil {
		return RetrievedData{}, err
	}

	isText := false
	returnedMime := ""
	if isMime {
		// Trim any trailing zero bytes from the MIME type in the payload header.
		mimeTypeBytes := bytes.TrimRight(payloadHeader, "\x00")

		returnedMime = string(mimeTypeBytes)
		isText = strings.HasPrefix(returnedMime, "text/")
	}

	parent := data.Parent
	if indexedParent, err := kv.GetParent(key); err == nil && !indexedParent.IsZero() {
		parent = indexedParent
	}

	metadataChildren := make([]hash.Hash, 0, len(data.Children))
	for _, child := range data.Children {
		if !child.IsZero() {
			metadataChildren = append(metadataChildren, child)
		}
	}

	indexedChildren, err := kv.GetChildren(key)
	if err != nil {
		return RetrievedData{}, err
	}

	children := mergeHashSets(metadataChildren, indexedChildren)

	var createdAt time.Time
	if len(data.MetaData) > 0 {
		meta, metaErr := decodeMetadata(data.MetaData)
		if metaErr != nil {
			if ou.log != nil {
				ou.log.Warn("failed to decode metadata", "error", metaErr, "key", key.String())
			}
		} else {
			createdAt = meta.CreatedAt
		}
	}

	return RetrievedData{
		Key:       key,
		Content:   content,
		MimeType:  returnedMime,
		IsText:    isText,
		Parent:    parent,
		Children:  children,
		CreatedAt: createdAt,
	}, nil
}

type storedMetadata struct {
	CreatedAt time.Time `json:"created_at"`
}

func encodeMetadata(meta storedMetadata) ([]byte, error) {
	return json.Marshal(meta)
}

func decodeMetadata(raw []byte) (storedMetadata, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return storedMetadata{}, nil
	}

	var meta storedMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return storedMetadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	return meta, nil
}

// encodeContentWithMimeType encodes raw content with an optional MIME type header.
//
// If mimeType is empty or contains only whitespace, the function returns a new byte slice
// that starts with a single header byte set to payloadHeaderNotExisting followed by the content.
//
// If mimeType is non-empty, it is trimmed of surrounding whitespace and validated against
// payloadHeaderContentSizeLen; if it exceeds that length the function returns an error.
// Otherwise the function builds a header of size payloadHeaderSize with the first byte
// set to payloadHeaderIsMime and the MIME type bytes copied into header[1:]. The returned
// payload is the header concatenated with the content bytes.
//
// The function does not modify its input slices and returns either the encoded payload or an error.
func encodeContentWithMimeType(content []byte, mimeType string) ([]byte, error) {

	// check if mimeType is empty before TrimSpace to avoid unnecessary processing
	if mimeType == "" {
		encoded := append(make([]byte, 1), content...)
		encoded[0] = payloadHeaderNotExisting
		return encoded, nil
	}

	cleanMimeType := strings.TrimSpace(mimeType)
	if cleanMimeType == "" {
		encoded := append(make([]byte, 1), content...)
		encoded[0] = payloadHeaderNotExisting
		return encoded, nil
	}

	mimeBytes := []byte(cleanMimeType)
	if len(mimeBytes) > payloadHeaderContentSizeLen {
		return nil, fmt.Errorf("MIME type too long: %d bytes (max %d)", len(mimeBytes), payloadHeaderContentSizeLen)
	}
	header := make([]byte, payloadHeaderSize)

	header[0] = payloadHeaderIsMime
	copy(header[1:], mimeBytes)

	return append(header, content...), nil
}

// decodeContent parses the supplied payload into the payload data, optional header and a MIME flag.
//
// The first byte of the payload is a flag byte indicating the presence of a payload header and whether it contains a MIME type.
// If the flag indicates no payload header, the function returns the content starting from payload[1:].
// If the flag indicates a payload header with MIME type, the function extracts the header from payload[1:payloadHeaderSize]
// and returns the remaining bytes as content. If the flag combination is invalid or the payload is too short,
// an error is returned.
//
// The function does not modify its input slice and returns either the decoded content, header, MIME flag, or an error.
func (ou *OuroborosDB) decodeContent(payload []byte) (data []byte, payloadHeader []byte, isMime bool, err error) {
	if len(payload) < 1 {
		return nil, nil, false, errors.New("ouroboros: payload is impossible short, it must be at least 1 byte")
	}

	ou.log.Debug("Decoding payload", "length", len(payload), "flag", payload[0])

	// No payload header is set
	if payload[0] == payloadHeaderNotExisting {
		return payload[1:], nil, false, nil
	}

	ou.log.Debug("Decoding payload", "length", len(payload), "flag", payload[0])

	// If not valid flag for existing data with MIME type, return error
	if payload[0] != payloadHeaderExisting && payload[0] != payloadHeaderIsMime {
		return []byte{}, nil, false, errors.New("ouroboros: invalid payload header flag combination")
	}
	if len(payload) < payloadHeaderSize {
		return nil, nil, false, errors.New("ouroboros: payloadHeader indicated but payload too short")
	}

	payloadHeader = bytes.TrimRight(payload[1:payloadHeaderSize], "\x00")
	data = payload[payloadHeaderSize:]
	isMime = payload[0] == payloadHeaderIsMime

	return data, payloadHeader, isMime, nil
}
