package apiServer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
)

func TestCreateAndList(t *testing.T) { // A
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db, WithLogger(testLogger()))

	payload := []byte("hello ouroboros")
	metadata := map[string]any{
		"mime_type": "text/plain; charset=utf-8",
	}

	req := newMultipartRequest(t, http.MethodPost, "/data", payload, "message.txt", "text/plain; charset=utf-8", metadata)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var createResp struct {
		Key string `json:"key"`
	}
	decodeJSONResponse(t, rec, &createResp)
	if createResp.Key == "" {
		t.Fatalf("expected key in response")
	}

	listRec := httptest.NewRecorder()
	server.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/data", nil))
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, listRec.Code)
	}

	var list listResponse
	decodeJSONResponse(t, listRec, &list)
	if len(list.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(list.Keys))
	}
	if list.Keys[0] != createResp.Key {
		t.Fatalf("expected returned key to match create response")
	}

	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/data/"+createResp.Key, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	body := getRec.Body.Bytes()
	if string(body) != string(payload) {
		t.Fatalf("expected retrieved content to match original")
	}
	if ct := getRec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected content type to be text/plain, got %q", ct)
	}
	if isText := getRec.Header().Get("X-Ouroboros-Is-Text"); isText != "true" {
		t.Fatalf("expected is-text header to be true, got %q", isText)
	}
	if mime := getRec.Header().Get("X-Ouroboros-Mime"); mime != "text/plain; charset=utf-8" {
		t.Fatalf("expected X-Ouroboros-Mime to match, got %q", mime)
	}
	created := getRec.Header().Get("X-Ouroboros-Created-At")
	if created == "" {
		t.Fatal("expected created timestamp header to be set")
	}
	if _, err := time.Parse(time.RFC3339Nano, created); err != nil {
		t.Fatalf("expected created timestamp to parse, got %q: %v", created, err)
	}
}

func TestCreateValidation(t *testing.T) { // A
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	req := newMultipartRequest(t, http.MethodPost, "/data", nil, "", "", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestGetBinaryData(t *testing.T) { // A
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	metadata := map[string]any{
		"mime_type": "application/octet-stream",
	}

	req := newMultipartRequest(t, http.MethodPost, "/data", payload, "payload.bin", "application/octet-stream", metadata)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var createResp struct {
		Key string `json:"key"`
	}
	decodeJSONResponse(t, rec, &createResp)
	if createResp.Key == "" {
		t.Fatalf("expected key in response")
	}

	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/data/"+createResp.Key, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	if ct := getRec.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("expected binary content type, got %q", ct)
	}
	if isText := getRec.Header().Get("X-Ouroboros-Is-Text"); isText != "false" {
		t.Fatalf("expected is-text header to be false, got %q", isText)
	}
	if mime := getRec.Header().Get("X-Ouroboros-Mime"); mime != "application/octet-stream" {
		t.Fatalf("expected X-Ouroboros-Mime to match, got %q", mime)
	}
	created := getRec.Header().Get("X-Ouroboros-Created-At")
	if created == "" {
		t.Fatal("expected created timestamp header to be set for binary data")
	}
	if _, err := time.Parse(time.RFC3339Nano, created); err != nil {
		t.Fatalf("expected created timestamp to parse for binary data, got %q: %v", created, err)
	}

	if !bytes.Equal(getRec.Body.Bytes(), payload) {
		t.Fatalf("expected binary content to match original")
	}
}

func TestCreateTextWithExplicitMIME(t *testing.T) { // A
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	payload := []byte("hello world")
	metadata := map[string]any{
		"mime_type": "text/plain; charset=utf-8",
	}

	req := newMultipartRequest(t, http.MethodPost, "/data", payload, "note.txt", "text/plain; charset=utf-8", metadata)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var createResp struct {
		Key string `json:"key"`
	}
	decodeJSONResponse(t, rec, &createResp)
	if createResp.Key == "" {
		t.Fatalf("expected key in response")
	}

	getRec := httptest.NewRecorder()
	server.ServeHTTP(getRec, httptest.NewRequest(http.MethodGet, "/data/"+createResp.Key, nil))
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, getRec.Code)
	}

	if ct := getRec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text content type, got %q", ct)
	}
	if isText := getRec.Header().Get("X-Ouroboros-Is-Text"); isText != "true" {
		t.Fatalf("expected is-text header to be true, got %q", isText)
	}
	if mime := getRec.Header().Get("X-Ouroboros-Mime"); mime != "text/plain; charset=utf-8" {
		t.Fatalf("expected X-Ouroboros-Mime to match, got %q", mime)
	}

	if getRec.Body.String() != string(payload) {
		t.Fatalf("expected text content to match original")
	}
}

func decodeJSONResponse(t *testing.T, rec *httptest.ResponseRecorder, target any) { // A
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), target); err != nil {
		t.Fatalf("failed to decode response: %v (body: %s)", err, rec.Body.String())
	}
}

func newMultipartRequest(t *testing.T, method, target string, payload []byte, filename, mimeType string, metadata map[string]any) *http.Request { // A
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if filename != "" {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		header.Set("Content-Type", mimeType)

		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("failed to create file part: %v", err)
		}
		if _, err := part.Write(payload); err != nil {
			t.Fatalf("failed to write payload: %v", err)
		}
	}

	if len(metadata) > 0 {
		metaJSON, err := json.Marshal(metadata)
		if err != nil {
			t.Fatalf("failed to marshal metadata: %v", err)
		}
		if err := writer.WriteField("metadata", string(metaJSON)); err != nil {
			t.Fatalf("failed to write metadata field: %v", err)
		}
	}

	contentType := writer.FormDataContentType()
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", contentType)
	return req
}

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestDB(t *testing.T) (*ouroboros.OuroborosDB, func()) { // A
	t.Helper()

	dir, err := os.MkdirTemp("", "ouroboros_api_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	keyPath := filepath.Join(dir, "ouroboros.key")

	asyncCrypt, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("failed to create async crypt: %v", err)
	}

	if err := asyncCrypt.SaveToFile(keyPath); err != nil {
		t.Fatalf("failed to save key file: %v", err)
	}

	cfg := ouroboros.Config{
		Paths:         []string{dir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db, err := ouroboros.New(cfg)
	if err != nil {
		t.Fatalf("failed to instantiate db: %v", err)
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("failed to start db: %v", err)
	}

	cleanup := func() {
		if err := db.CloseWithoutContext(); err != nil {
			t.Errorf("failed to close db: %v", err)
		}
		os.RemoveAll(dir)
	}

	return db, cleanup
}

type apiHarness struct {
	t         *testing.T
	server    http.Handler
	authCalls *int
}

func newAPIHarness(t *testing.T, auth AuthFunc, opts ...Option) *apiHarness { // A
	t.Helper()

	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	var counter int
	baseOpts := []Option{
		WithLogger(testLogger()),
		WithAuth(func(r *http.Request) error {
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
		server:    New(db, baseOpts...),
		authCalls: &counter,
	}
}

func (h *apiHarness) request(method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder { // A
	h.t.Helper()

	req := httptest.NewRequest(method, target, body)
	for key, value := range headers {
		if value == "" {
			continue
		}
		req.Header.Set(key, value)
	}

	recorder := httptest.NewRecorder()
	h.server.ServeHTTP(recorder, req)
	return recorder
}

func (h *apiHarness) authCount() int { // A
	if h.authCalls == nil {
		return 0
	}
	return *h.authCalls
}

func (h *apiHarness) requireStatus(rec *httptest.ResponseRecorder, expected int) { // A
	h.t.Helper()
	if rec.Code != expected {
		h.t.Fatalf("expected status %d, got %d, body: %s", expected, rec.Code, rec.Body.String())
	}
}

type createConfig struct {
	mimeType string
	parent   string
	children []string
	shards   uint8
	parity   uint8
	filename string
}

type CreateOption func(*createConfig)

func WithMimeType(mime string) CreateOption { // A
	return func(cfg *createConfig) {
		cfg.mimeType = mime
	}
}

func WithParent(parent string) CreateOption { // A
	return func(cfg *createConfig) {
		cfg.parent = parent
	}
}

func WithChildren(children ...string) CreateOption { // A
	return func(cfg *createConfig) {
		cfg.children = append([]string{}, children...)
	}
}

func WithReedSolomon(shards, parity uint8) CreateOption { // A
	return func(cfg *createConfig) {
		cfg.shards = shards
		cfg.parity = parity
	}
}

func WithFilename(name string) CreateOption { // A
	return func(cfg *createConfig) {
		cfg.filename = name
	}
}

func (h *apiHarness) create(content []byte, opts ...CreateOption) string { // A
	h.t.Helper()

	cfg := createConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	filename := cfg.filename
	if filename == "" {
		if strings.HasPrefix(strings.ToLower(cfg.mimeType), "text/") {
			filename = "payload.txt"
		} else {
			filename = "payload.bin"
		}
	}

	metadata := map[string]any{}
	if cfg.mimeType != "" {
		metadata["mime_type"] = cfg.mimeType
	}
	if cfg.parent != "" {
		metadata["parent"] = cfg.parent
	}
	if len(cfg.children) > 0 {
		metadata["children"] = append([]string{}, cfg.children...)
	}
	if cfg.shards != 0 {
		metadata["reed_solomon_shards"] = cfg.shards
	}
	if cfg.parity != 0 {
		metadata["reed_solomon_parity_shards"] = cfg.parity
	}

	req := newMultipartRequest(h.t, http.MethodPost, "/data", content, filename, cfg.mimeType, metadata)
	rec := httptest.NewRecorder()
	h.server.ServeHTTP(rec, req)
	h.requireStatus(rec, http.StatusCreated)

	var resp struct {
		Key string `json:"key"`
	}
	decodeJSONResponse(h.t, rec, &resp)
	if resp.Key == "" {
		h.t.Fatalf("expected create response to include key")
	}
	return resp.Key
}

func (h *apiHarness) list() []string { // A
	rec := h.request(http.MethodGet, "/data", nil, nil)
	h.requireStatus(rec, http.StatusOK)

	var resp struct {
		Keys []string `json:"keys"`
	}
	decodeJSONResponse(h.t, rec, &resp)
	return resp.Keys
}

func (h *apiHarness) get(key string) ([]byte, http.Header) { // A
	rec := h.request(http.MethodGet, "/data/"+key, nil, nil)
	h.requireStatus(rec, http.StatusOK)

	res := rec.Result()
	defer res.Body.Close()

	body := append([]byte(nil), rec.Body.Bytes()...)
	header := res.Header.Clone()
	return body, header
}

func (h *apiHarness) options(path string, headers map[string]string) *httptest.ResponseRecorder { // A
	return h.request(http.MethodOptions, path, nil, headers)
}

func TestAPIServerCreate(t *testing.T) { // A
	h := newAPIHarness(t, nil)

	key := h.create([]byte("hello ouroboros api"), WithMimeType("text/plain; charset=utf-8"))
	if key == "" {
		t.Fatalf("expected key to be returned")
	}

	if h.authCount() != 1 {
		t.Fatalf("expected auth hook to run once, ran %d", h.authCount())
	}
}

func TestAPIServerListAfterCreate(t *testing.T) { // A
	h := newAPIHarness(t, nil)

	key := h.create([]byte("list me"), WithMimeType("text/plain; charset=utf-8"))
	keys := h.list()

	if len(keys) != 1 || keys[0] != key {
		t.Fatalf("expected list to return created key, got %v", keys)
	}

	if h.authCount() != 2 {
		t.Fatalf("expected auth hook to run twice, ran %d", h.authCount())
	}
}

func TestAPIServerGetAfterCreate(t *testing.T) { // A
	h := newAPIHarness(t, nil)

	payload := []byte("fetch me")
	key := h.create(payload, WithMimeType("text/plain; charset=utf-8"))

	content, header := h.get(key)
	if string(content) != string(payload) {
		t.Fatalf("expected retrieved content to match original")
	}
	if header.Get("X-Ouroboros-Is-Text") != "true" {
		t.Fatalf("expected retrieved data to be marked as text")
	}
	if header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("expected content type to be text/plain, got %q", header.Get("Content-Type"))
	}
	if header.Get("X-Ouroboros-Mime") != "text/plain; charset=utf-8" {
		t.Fatalf("expected X-Ouroboros-Mime to match, got %q", header.Get("X-Ouroboros-Mime"))
	}

	if h.authCount() != 2 {
		t.Fatalf("expected auth hook to run twice, ran %d", h.authCount())
	}
}

func TestAPIServerOptionsSkipsAuth(t *testing.T) { // A
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

func TestAPIServerAuthFailure(t *testing.T) { // A
	expectedErr := fmt.Errorf("no credentials")
	h := newAPIHarness(t, func(r *http.Request) error { return expectedErr })

	rec := h.request(http.MethodGet, "/data", nil, nil)

	if status := rec.Code; status != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, status)
	}

	if h.authCount() != 1 {
		t.Fatalf("expected auth hook to run once, ran %d", h.authCount())
	}
}

func TestAPIServerParentChildHeaders(t *testing.T) { // A
	h := newAPIHarness(t, nil)

	parentKey := h.create([]byte("parent"), WithMimeType("text/plain; charset=utf-8"))
	childKey := h.create([]byte("child"), WithMimeType("text/plain; charset=utf-8"), WithParent(parentKey))

	_, childHeader := h.get(childKey)
	if headerParent := childHeader.Get("X-Ouroboros-Parent"); headerParent != parentKey {
		t.Fatalf("expected child header parent %s, got %s", parentKey, headerParent)
	}

	_, parentHeader := h.get(parentKey)
	childrenHeader := parentHeader.Get("X-Ouroboros-Children")
	if childrenHeader == "" {
		t.Fatalf("expected parent header to include children")
	}

	hasChild := false
	for _, value := range strings.Split(childrenHeader, ",") {
		if strings.TrimSpace(value) == childKey {
			hasChild = true
			break
		}
	}

	if !hasChild {
		t.Fatalf("expected parent header children to contain %s, got %s", childKey, childrenHeader)
	}

	if h.authCount() != 4 {
		t.Fatalf("expected auth hook to run four times, ran %d", h.authCount())
	}
}

func TestAPIServerChildrenEndpoint(t *testing.T) { // A
	h := newAPIHarness(t, nil)

	parentKey := h.create([]byte("parent"), WithMimeType("text/plain; charset=utf-8"))
	childKey := h.create([]byte("child"), WithMimeType("text/plain; charset=utf-8"), WithParent(parentKey))

	roots := h.list()
	if len(roots) != 1 || roots[0] != parentKey {
		t.Fatalf("expected list to return only parent root, got %v", roots)
	}

	rec := h.request(http.MethodGet, fmt.Sprintf("/data/%s/children", parentKey), nil, nil)
	h.requireStatus(rec, http.StatusOK)

	var resp struct {
		Keys []string `json:"keys"`
	}
	decodeJSONResponse(t, rec, &resp)
	if len(resp.Keys) != 1 || resp.Keys[0] != childKey {
		t.Fatalf("expected children endpoint to return child key, got %v", resp.Keys)
	}

	if h.authCount() != 4 {
		t.Fatalf("expected auth hook to run four times, ran %d", h.authCount())
	}
}
