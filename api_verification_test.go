package ouroboros_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
	api "github.com/i5heu/ouroboros-db/api"
)

// newTempStartedDB creates and starts an OuroborosDB instance backed by a
// temporary testing directory. The database is automatically closed when the
// test finishes.
func newTempStartedDB(t *testing.T) *ouroboros.OuroborosDB {
	t.Helper()

	dataDir := t.TempDir()

	crypt, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("failed to create async crypt: %v", err)
	}

	keyPath := filepath.Join(dataDir, "ouroboros.key")
	if err := crypt.SaveToFile(keyPath); err != nil {
		t.Fatalf("failed to save key file: %v", err)
	}

	conf := ouroboros.Config{
		Paths:         []string{dataDir},
		MinimumFreeGB: 1,
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	db, err := ouroboros.New(conf)
	if err != nil {
		t.Fatalf("failed to construct db: %v", err)
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("failed to start db: %v", err)
	}

	t.Cleanup(func() {
		if err := db.CloseWithoutContext(); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
	})

	return db
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type apiHarness struct {
	t         *testing.T
	server    http.Handler
	authCalls *int
}

func newAPIHarness(t *testing.T, auth api.AuthFunc, opts ...api.Option) *apiHarness {
	t.Helper()

	db := newTempStartedDB(t)

	var counter int
	baseOpts := []api.Option{
		api.WithLogger(testLogger()),
		api.WithAuth(func(r *http.Request) error {
			counter++
			if auth != nil {
				return auth(r)
			}
			return nil
		}),
	}

	baseOpts = append(baseOpts, opts...)

	return &apiHarness{
		t:         t,
		server:    api.New(db, baseOpts...),
		authCalls: &counter,
	}
}

func (h *apiHarness) request(method, target string, body any, headers map[string]string) *httptest.ResponseRecorder {
	h.t.Helper()

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("failed to marshal request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, target, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	h.server.ServeHTTP(recorder, req)
	return recorder
}

func (h *apiHarness) authCount() int {
	if h.authCalls == nil {
		return 0
	}
	return *h.authCalls
}

func (h *apiHarness) requireStatus(rec *httptest.ResponseRecorder, expected int) {
	h.t.Helper()
	if rec.Code != expected {
		h.t.Fatalf("expected status %d, got %d, body: %s", expected, rec.Code, rec.Body.String())
	}
}

func (h *apiHarness) decode(rec *httptest.ResponseRecorder, target any) {
	h.t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(target); err != nil {
		h.t.Fatalf("failed to decode response: %v (body: %s)", err, rec.Body.String())
	}
}

type createConfig struct {
	mimeType string
	parent   string
	children []string
	shards   uint8
	parity   uint8
}

type CreateOption func(*createConfig)

func WithMimeType(mime string) CreateOption {
	return func(cfg *createConfig) {
		cfg.mimeType = mime
	}
}

func WithParent(parent string) CreateOption {
	return func(cfg *createConfig) {
		cfg.parent = parent
	}
}

func WithChildren(children ...string) CreateOption {
	return func(cfg *createConfig) {
		cfg.children = append([]string{}, children...)
	}
}

func WithReedSolomon(shards, parity uint8) CreateOption {
	return func(cfg *createConfig) {
		cfg.shards = shards
		cfg.parity = parity
	}
}

func (h *apiHarness) create(content []byte, opts ...CreateOption) string {
	h.t.Helper()

	cfg := createConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	body := map[string]any{
		"content": base64.StdEncoding.EncodeToString(content),
	}
	if cfg.mimeType != "" {
		body["mime_type"] = cfg.mimeType
	}
	if cfg.parent != "" {
		body["parent"] = cfg.parent
	}
	if len(cfg.children) > 0 {
		body["children"] = cfg.children
	}
	if cfg.shards != 0 {
		body["reed_solomon_shards"] = cfg.shards
	}
	if cfg.parity != 0 {
		body["reed_solomon_parity_shards"] = cfg.parity
	}

	rec := h.request(http.MethodPost, "/data", body, nil)
	h.requireStatus(rec, http.StatusCreated)

	var resp struct {
		Key string `json:"key"`
	}
	h.decode(rec, &resp)
	if resp.Key == "" {
		h.t.Fatalf("expected create response to include key")
	}
	return resp.Key
}

func (h *apiHarness) list() []string {
	rec := h.request(http.MethodGet, "/data", nil, nil)
	h.requireStatus(rec, http.StatusOK)

	var resp struct {
		Keys []string `json:"keys"`
	}
	h.decode(rec, &resp)
	return resp.Keys
}

func (h *apiHarness) get(key string) (string, string, bool) {
	rec := h.request(http.MethodGet, "/data/"+key, nil, nil)
	h.requireStatus(rec, http.StatusOK)

	var resp struct {
		Key     string `json:"key"`
		Content string `json:"content"`
		IsText  bool   `json:"is_text"`
		Mime    string `json:"mime_type"`
	}
	h.decode(rec, &resp)
	return resp.Content, resp.Mime, resp.IsText
}

func (h *apiHarness) options(path string, headers map[string]string) *httptest.ResponseRecorder {
	return h.request(http.MethodOptions, path, nil, headers)
}

func TestAPIServerCreate(t *testing.T) {
	h := newAPIHarness(t, nil)

	key := h.create([]byte("hello ouroboros api"))
	if key == "" {
		t.Fatalf("expected key to be returned")
	}

	if h.authCount() != 1 {
		t.Fatalf("expected auth hook to run once, ran %d", h.authCount())
	}
}

func TestAPIServerListAfterCreate(t *testing.T) {
	h := newAPIHarness(t, nil)

	key := h.create([]byte("list me"))
	keys := h.list()

	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("expected list to return created key, got %v", keys)
	}

	if h.authCount() != 2 {
		t.Fatalf("expected auth hook to run twice, ran %d", h.authCount())
	}
}

func TestAPIServerGetAfterCreate(t *testing.T) {
	h := newAPIHarness(t, nil)

	payload := []byte("fetch me")
	key := h.create(payload)

	contentBase64, mimeType, isText := h.get(key)
	if !isText {
		t.Fatalf("expected retrieved data to be marked as text")
	}
	if mimeType != "" {
		t.Fatalf("expected mime type to be empty, got %q", mimeType)
	}

	decoded, err := base64.StdEncoding.DecodeString(contentBase64)
	if err != nil {
		t.Fatalf("failed to decode response content: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("expected retrieved content to match original")
	}

	if h.authCount() != 2 {
		t.Fatalf("expected auth hook to run twice, ran %d", h.authCount())
	}
}

func TestAPIServerOptionsSkipsAuth(t *testing.T) {
	h := newAPIHarness(t, nil)

	rec := h.options("/data", map[string]string{"Origin": "https://example.test"})

	if status := rec.Code; status != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, status)
	}
	if allowOrigin := rec.Header().Get("Access-Control-Allow-Origin"); allowOrigin != "https://example.test" {
		t.Fatalf("expected CORS header to echo origin, got %q", allowOrigin)
	}

	if h.authCount() != 0 {
		t.Fatalf("expected auth hook to be skipped for OPTIONS, ran %d", h.authCount())
	}
}

func TestAPIServerAuthFailure(t *testing.T) {
	expectedErr := errors.New("no credentials")
	h := newAPIHarness(t, func(r *http.Request) error { return expectedErr })

	rec := h.request(http.MethodGet, "/data", nil, nil)

	if status := rec.Code; status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}

	if h.authCount() != 1 {
		t.Fatalf("expected auth hook to run once, ran %d", h.authCount())
	}
}
