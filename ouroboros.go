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
	"github.com/i5heu/ouroboros-crypt/pkg/hash"

	// MIME type previously encoded in content is now stored in metadata.
	indexpkg "github.com/i5heu/ouroboros-db/pkg/index"
	meta "github.com/i5heu/ouroboros-db/pkg/meta"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

// OuroborosDB is the main database handle. It owns the crypt layer, the KV
// store, and the lifecycle of background components.
type OuroborosDB struct {
	log    *slog.Logger
	config Config

	crypt   *crypt.Crypt
	kvMu    sync.RWMutex
	kv      ouroboroskv.Store
	indexer *indexpkg.Indexer

	started   atomic.Bool
	startOnce sync.Once
	closeOnce sync.Once
}

// Indexer returns the internal indexer instance. Mainly used for integration or
// in tests to interact with the index.
func (ou *OuroborosDB) Indexer() *indexpkg.Indexer {
	// It's safe to return the pointer directly.
	return ou.indexer
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
	Title                   string
	EditOf                  hash.Hash
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
	// UiPort specifies the port for the built-in web UI. If 0, the UI is disabled.
	UiPort uint16
}

type RetrievedData struct {
	Key         hash.Hash
	ResolvedKey hash.Hash
	EditOf      hash.Hash
	Content     []byte
	MimeType    string
	IsText      bool
	Parent      hash.Hash
	Children    []hash.Hash
	CreatedAt   time.Time
	Title       string
}

// Use shared metadata type from pkg/meta
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
		ou.kvMu.Lock()
		ou.kv = kv
		ou.kvMu.Unlock()

		// Initialize indexer to keep a Bleve index synchronized with the KV content.
		ou.indexer = indexpkg.NewIndexer(kv, ou.log)
		// Populate index synchronously on startup; do not run this in the background
		// to avoid races with DB close during tests and short-lived instances.
		if err := ou.indexer.ReindexAll(); err != nil {
			ou.log.Warn("indexer reindex failed", "error", err)
		}

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
		ou.kvMu.Lock()
		kv := ou.kv
		ou.kv = nil
		ou.kvMu.Unlock()
		if kv != nil {
			if err := kv.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("close kv: %w", err))
			}
		}
		// Close indexer if it exists
		if ou.indexer != nil {
			if err := ou.indexer.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("close indexer: %w", err))
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

func (ou *OuroborosDB) kvHandle() (ouroboroskv.Store, error) { // A
	if !ou.started.Load() {
		return nil, ErrNotStarted
	}

	ou.kvMu.RLock()
	kv := ou.kv
	ou.kvMu.RUnlock()
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

	// Store content raw. MimeType lives in the metadata JSON instead of being encoded into the payload.
	encodedContent := content

	// TODO allow additional user-defined metadata but server set CreatedAt is mandatory
	md := meta.Metadata{CreatedAt: time.Now().UTC(), Title: opts.Title, MimeType: opts.MimeType}
	if !opts.EditOf.IsZero() {
		md.EditOf = opts.EditOf.String()
	}

	metaBytes, err := encodeMetadata(md)
	if err != nil {
		return hash.Hash{}, fmt.Errorf("encode metadata: %w", err)
	}

	data := ouroboroskv.Data{
		Meta:           metaBytes,
		Content:        encodedContent,
		RSDataSlices:   opts.ReedSolomonShards,
		RSParitySlices: opts.ReedSolomonParityShards,
	}

	effectiveParent := opts.Parent
	if !opts.EditOf.IsZero() {
		effectiveParent = opts.EditOf
	}
	if !effectiveParent.IsZero() {
		data.Parent = effectiveParent
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

	// After successful write, index the new data asynchronously if indexer is present.
	if ou.indexer != nil {
		go func(h hash.Hash) {
			_ = ou.indexer.IndexHash(h)
		}(dataHash)
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

	resolvedKey, resolvedData, lineage, err := ou.resolveLatestEdit(ctx, kv, key)
	if err != nil {
		return RetrievedData{}, err
	}

	originalData := resolvedData
	if resolvedKey != key {
		if data, readErr := kv.ReadData(key); readErr == nil {
			originalData = data
		}
	}

	// Content is stored raw in the data payload and the MIME type is stored in metadata.
	content := resolvedData.Content
	isText := false
	returnedMime := ""
	var editOf hash.Hash

	parent := originalData.Parent
	if indexedParent, err := kv.GetParent(key); err == nil && !indexedParent.IsZero() {
		parent = indexedParent
	}
	if parent.IsZero() {
		parent = resolvedData.Parent
	}

	var createdAt time.Time
	var title string
	if len(resolvedData.Meta) > 0 {
		md, metaErr := decodeMetadata(resolvedData.Meta)
		if metaErr != nil {
			if ou.log != nil {
				ou.log.Warn("failed to decode metadata", "error", metaErr, "key", key.String())
			}
		} else {
			createdAt = md.CreatedAt
			title = md.Title
			if md.MimeType != "" {
				returnedMime = md.MimeType
				isText = strings.HasPrefix(returnedMime, "text/")
			}
			if parsedEdit, err := parseEditOf(md.EditOf); err == nil {
				editOf = parsedEdit
			}
		}
	}

	children := mergeChildrenExcludingEdits(kv, lineage, originalData.Children, resolvedData.Children)

	return RetrievedData{
		Key:         key,
		ResolvedKey: resolvedKey,
		EditOf:      editOf,
		Content:     content,
		MimeType:    returnedMime,
		IsText:      isText,
		Parent:      parent,
		Children:    children,
		CreatedAt:   createdAt,
		Title:       title,
	}, nil
}

func encodeMetadata(m meta.Metadata) ([]byte, error) { // PHC
	return json.Marshal(m)
}

// decodeMetadata parses JSON-encoded metadata from the given raw bytes into a meta.Metadata value.
//
// Empty or whitespace-only input is treated as absent metadata and results in a zero-value meta.Metadata and a nil error.
// On success it returns the decoded meta.Metadata and a nil error; if JSON decoding fails the error is returned wrapped with context ("decode metadata:").
func decodeMetadata(raw []byte) (meta.Metadata, error) { // PHC
	if len(raw) == 0 || len(bytes.TrimSpace(raw)) == 0 {
		return meta.Metadata{}, nil
	}

	var md meta.Metadata
	if err := json.Unmarshal(raw, &md); err != nil {
		return meta.Metadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	return md, nil
}

func parseEditOf(raw string) (hash.Hash, error) { // HC
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return hash.Hash{}, nil
	}
	return hash.HashHexadecimal(trimmed)
}

func (ou *OuroborosDB) resolveLatestEdit(ctx context.Context, kv ouroboroskv.Store, start hash.Hash) (hash.Hash, ouroboroskv.Data, []hash.Hash, error) { // PAP
	if err := ctx.Err(); err != nil {
		return hash.Hash{}, ouroboroskv.Data{}, nil, err
	}

	currentKey := start
	visited := make(map[string]struct{})
	lineage := make([]hash.Hash, 0, 4)
	var currentData ouroboroskv.Data

	for {
		if err := ctx.Err(); err != nil {
			return hash.Hash{}, ouroboroskv.Data{}, lineage, err
		}
		if _, seen := visited[currentKey.String()]; seen {
			return hash.Hash{}, ouroboroskv.Data{}, lineage, fmt.Errorf("edit chain cycle detected for %s", currentKey.String())
		}
		visited[currentKey.String()] = struct{}{}
		lineage = append(lineage, currentKey)

		data, err := kv.ReadData(currentKey)
		if err != nil {
			return hash.Hash{}, ouroboroskv.Data{}, lineage, err
		}
		currentData = data

		var latestKey hash.Hash
		var latestData ouroboroskv.Data
		var latestCreated time.Time

		for _, child := range data.Children {
			if child.IsZero() {
				continue
			}
			childData, err := kv.ReadData(child)
			if err != nil {
				continue
			}
			md, err := decodeMetadata(childData.Meta)
			if err != nil {
				continue
			}
			editTarget, err := parseEditOf(md.EditOf)
			if err != nil || editTarget.IsZero() {
				continue
			}
			if editTarget != currentKey {
				continue
			}
			createdAt := md.CreatedAt
			if latestKey.IsZero() || createdAt.After(latestCreated) {
				latestKey = child
				latestData = childData
				latestCreated = createdAt
			}
		}

		if latestKey.IsZero() {
			return currentKey, currentData, lineage, nil
		}

		currentKey = latestKey
		currentData = latestData
	}
}

func mergeChildrenExcludingEdits(kv ouroboroskv.Store, lineage []hash.Hash, childLists ...[]hash.Hash) []hash.Hash { // HC
	lineageSet := make(map[string]struct{}, len(lineage))
	for _, k := range lineage {
		lineageSet[k.String()] = struct{}{}
	}

	seen := make(map[string]struct{})
	merged := make([]hash.Hash, 0)

	for _, list := range childLists {
		for _, child := range list {
			if child.IsZero() {
				continue
			}
			keyStr := child.String()
			if _, exists := seen[keyStr]; exists {
				continue
			}

			if childData, err := kv.ReadData(child); err == nil {
				if md, err := decodeMetadata(childData.Meta); err == nil {
					if editTarget, err := parseEditOf(md.EditOf); err == nil && !editTarget.IsZero() {
						if _, isEditChild := lineageSet[editTarget.String()]; isEditChild {
							continue
						}
					}
				}
			}

			seen[keyStr] = struct{}{}
			merged = append(merged, child)
		}
	}

	return merged
}
