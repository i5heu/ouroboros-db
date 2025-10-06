/*
!! Currently the database is in a very early stage of development and should not be used in production environments. !!
*/
package ouroboros

import (
	"bytes"
	"context"
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
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

// OuroborosDB is the main database handle. It owns the crypt layer, the KV
// store, and the lifecycle of background components.
type OuroborosDB struct {
	log    *slog.Logger
	config Config

	crypt *crypt.Crypt
	kv    *ouroboroskv.KV

	started   atomic.Bool
	startOnce sync.Once
	closeOnce sync.Once
	mu        sync.Mutex
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

		ou.mu.Lock()
		ou.crypt = c
		ou.kv = kv
		ou.mu.Unlock()

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
		ou.mu.Lock()
		defer ou.mu.Unlock()

		if ou.kv != nil {
			if err := ou.kv.Close(); err != nil {
				closeErr = errors.Join(closeErr, fmt.Errorf("close kv: %w", err))
			}
			ou.kv = nil
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

const (
	payloadHeaderSize    = 256
	payloadHeaderText    = 0x80
	payloadHeaderMIMELen = payloadHeaderSize - 1
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

func (ou *OuroborosDB) getComponents() (*ouroboroskv.KV, *crypt.Crypt, error) {
	if !ou.started.Load() {
		return nil, nil, ErrNotStarted
	}

	ou.mu.Lock()
	kv := ou.kv
	c := ou.crypt
	ou.mu.Unlock()

	if kv == nil || c == nil {
		return nil, nil, ErrClosed
	}

	return kv, c, nil
}

func (ou *OuroborosDB) StoreData(ctx context.Context, content []byte, opts StoreOptions) (hash.Hash, error) {
	if err := ctx.Err(); err != nil {
		return hash.Hash{}, err
	}

	kv, cryptLayer, err := ou.getComponents()
	if err != nil {
		return hash.Hash{}, err
	}

	opts.applyDefaults()

	encodedContent, err := encodeContent(content, opts.MimeType)
	if err != nil {
		return hash.Hash{}, err
	}

	key := cryptLayer.HashBytes(encodedContent)

	data := ouroboroskv.Data{
		Key:                     key,
		Content:                 encodedContent,
		ReedSolomonShards:       opts.ReedSolomonShards,
		ReedSolomonParityShards: opts.ReedSolomonParityShards,
	}

	if !opts.Parent.IsZero() {
		data.Parent = opts.Parent
	}
	for _, child := range opts.Children {
		if !child.IsZero() {
			data.Children = append(data.Children, child)
		}
	}

	if err := kv.WriteData(data); err != nil {
		return hash.Hash{}, err
	}

	return key, nil
}

func (ou *OuroborosDB) ListData(ctx context.Context) ([]hash.Hash, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	kv, _, err := ou.getComponents()
	if err != nil {
		return nil, err
	}

	keys, err := kv.ListKeys()
	if err != nil {
		return nil, err
	}

	return keys, nil
}

type RetrievedData struct {
	Key      hash.Hash
	Content  []byte
	MimeType string
	IsText   bool
	Parent   hash.Hash
	Children []hash.Hash
}

func (ou *OuroborosDB) GetData(ctx context.Context, key hash.Hash) (RetrievedData, error) {
	if err := ctx.Err(); err != nil {
		return RetrievedData{}, err
	}

	kv, _, err := ou.getComponents()
	if err != nil {
		return RetrievedData{}, err
	}

	data, err := kv.ReadData(key)
	if err != nil {
		return RetrievedData{}, err
	}

	content, mime, isText, err := decodeContent(data.Content)
	if err != nil {
		return RetrievedData{}, err
	}

	return RetrievedData{
		Key:      key,
		Content:  content,
		MimeType: mime,
		IsText:   isText,
		Parent:   data.Parent,
		Children: data.Children,
	}, nil
}

func encodeContent(content []byte, mimeType string) ([]byte, error) {
	header := make([]byte, payloadHeaderSize)

	trimmed := strings.TrimSpace(mimeType)

	if trimmed == "" {
		header[0] = payloadHeaderText
	} else {
		mimeBytes := []byte(trimmed)
		if len(mimeBytes) > payloadHeaderMIMELen {
			mimeBytes = mimeBytes[:payloadHeaderMIMELen]
		}
		copy(header[1:], mimeBytes)

		if isTextLikeMIME(trimmed) {
			header[0] = header[0] | payloadHeaderText
		}
	}

	encoded := make([]byte, len(header)+len(content))
	copy(encoded, header)
	copy(encoded[len(header):], content)
	return encoded, nil
}

func decodeContent(payload []byte) ([]byte, string, bool, error) {
	if len(payload) < payloadHeaderSize {
		return nil, "", false, ErrInvalidData
	}

	flag := payload[0]
	isText := flag&payloadHeaderText != 0

	raw := bytes.TrimRight(payload[1:payloadHeaderSize], "\x00")
	mime := strings.TrimSpace(string(raw))

	content := make([]byte, len(payload)-payloadHeaderSize)
	copy(content, payload[payloadHeaderSize:])
	return content, mime, isText, nil
}

func isTextLikeMIME(value string) bool {
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "text/") {
		return true
	}

	switch {
	case strings.Contains(lower, "json"), strings.Contains(lower, "xml"), strings.Contains(lower, "yaml"), strings.Contains(lower, "csv"):
		return true
	default:
		return false
	}
}
