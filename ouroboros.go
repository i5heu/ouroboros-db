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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/hash"
	"github.com/i5heu/ouroboros-db/encoding"
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

type StoreOptions struct {
	Parent                  hash.Hash
	Children                []hash.Hash
	ReedSolomonShards       uint8
	ReedSolomonParityShards uint8
	MimeType                string
}

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

type RetrievedData struct {
	Key       hash.Hash
	Content   []byte
	MimeType  string
	IsText    bool
	Parent    hash.Hash
	Children  []hash.Hash
	CreatedAt time.Time
}

type storedMetadata struct {
	CreatedAt time.Time `json:"created_at"`
}

// defaultLogger returns a logger that writes text logs to stderr at Info level.
// Applications can inject their own slog.Logger for JSON, different levels, etc.
func defaultLogger() *slog.Logger { // A
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(h)
}

// New constructs a database handle. New does not perform heavy I/O or start
// background goroutines. Call Start to initialize subsystems.
func New(conf Config) (*OuroborosDB, error) { // A
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
func (ou *OuroborosDB) Start(ctx context.Context) error { // PA
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
func (ou *OuroborosDB) Run(ctx context.Context) error { // A
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
func (ou *OuroborosDB) Close(ctx context.Context) error { // A
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
func (ou *OuroborosDB) CloseWithoutContext() error { // A
	return ou.Close(context.Background())
}

func (ou *OuroborosDB) kvHandle() (*ouroboroskv.KV, error) { // A
	if !ou.started.Load() {
		return nil, ErrNotStarted
	}

	kv := ou.kv.Load()
	if kv == nil {
		return nil, ErrClosed
	}

	return kv, nil
}

func (opts *StoreOptions) applyDefaults() { // HC
	if opts.ReedSolomonShards == 0 {
		opts.ReedSolomonShards = 4
	}
	if opts.ReedSolomonParityShards == 0 {
		opts.ReedSolomonParityShards = 2
	}
}

func (ou *OuroborosDB) StoreData(ctx context.Context, content []byte, opts StoreOptions) (hash.Hash, error) { // PAP
	if err := ctx.Err(); err != nil {
		return hash.Hash{}, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return hash.Hash{}, err
	}

	opts.applyDefaults()

	encodedContent, err := encoding.EncodeContentWithMimeType(content, opts.MimeType)
	if err != nil {
		return hash.Hash{}, err
	}

	// TODO allow additional user-defined metadata but server set CreatedAt is mandatory
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

func (ou *OuroborosDB) ListData(ctx context.Context) ([]hash.Hash, error) { //AC
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

func (ou *OuroborosDB) ListChildren(ctx context.Context, parent hash.Hash) ([]hash.Hash, error) { //HC
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	kv, err := ou.kvHandle()
	if err != nil {
		return nil, err
	}

	return kv.GetChildren(parent)
}

func (ou *OuroborosDB) GetData(ctx context.Context, key hash.Hash) (RetrievedData, error) { // PAC
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

	content, payloadHeader, isMime, err := encoding.DecodeContent(data.Content)
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
		Children:  data.Children,
		CreatedAt: createdAt,
	}, nil
}

func encodeMetadata(meta storedMetadata) ([]byte, error) { // PHC
	return json.Marshal(meta)
}

// decodeMetadata parses JSON-encoded metadata from the given raw bytes into a storedMetadata value.
//
// Empty or whitespace-only input is treated as absent metadata and results in a zero-value storedMetadata and a nil error.
// On success it returns the decoded storedMetadata and a nil error; if JSON decoding fails the error is returned wrapped with context ("decode metadata:").
func decodeMetadata(raw []byte) (storedMetadata, error) { // PHC
	if len(raw) == 0 || len(bytes.TrimSpace(raw)) == 0 {
		return storedMetadata{}, nil
	}

	var meta storedMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		return storedMetadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	return meta, nil
}
