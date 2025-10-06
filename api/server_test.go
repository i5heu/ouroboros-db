package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"log/slog"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
)

func TestCreateAndList(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db, WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))

	payload := []byte("hello ouroboros")
	reqBody := map[string]any{
		"content": base64.StdEncoding.EncodeToString(payload),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/data", bytes.NewReader(body))
	server.ServeHTTP(recorder, request)

	if status := recorder.Result().StatusCode; status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, status)
	}

	var create struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(recorder.Result().Body).Decode(&create); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	if create.Key == "" {
		t.Fatalf("expected key in response")
	}

	listRecorder := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/data", nil)
	server.ServeHTTP(listRecorder, listRequest)

	if status := listRecorder.Result().StatusCode; status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}

	var list listResponse
	if err := json.NewDecoder(listRecorder.Result().Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	if len(list.Keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(list.Keys))
	}

	if list.Keys[0] != create.Key {
		t.Fatalf("expected returned key to match create response")
	}

	dataRecorder := httptest.NewRecorder()
	dataRequest := httptest.NewRequest(http.MethodGet, "/data/"+create.Key, nil)
	server.ServeHTTP(dataRecorder, dataRequest)

	if status := dataRecorder.Result().StatusCode; status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}

	var data struct {
		Key      string `json:"key"`
		Content  string `json:"content"`
		MimeType string `json:"mime_type"`
		IsText   bool   `json:"is_text"`
	}
	if err := json.NewDecoder(dataRecorder.Result().Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode data response: %v", err)
	}

	if !data.IsText {
		t.Fatalf("expected data to be marked as text")
	}
	if data.MimeType != "" {
		t.Fatalf("expected empty mime type for text data, got %q", data.MimeType)
	}

	decoded, err := base64.StdEncoding.DecodeString(data.Content)
	if err != nil {
		t.Fatalf("failed to decode content: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("expected retrieved content to match original")
	}
}

func TestCreateValidation(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/data", bytes.NewReader([]byte(`{"content": ""}`)))
	server.ServeHTTP(recorder, request)

	if status := recorder.Result().StatusCode; status != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, status)
	}
}

func TestGetBinaryData(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	reqBody := map[string]any{
		"content":   base64.StdEncoding.EncodeToString(payload),
		"mime_type": "application/octet-stream",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/data", bytes.NewReader(body))
	server.ServeHTTP(recorder, request)

	if status := recorder.Result().StatusCode; status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, status)
	}

	var create struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(recorder.Result().Body).Decode(&create); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	dataRecorder := httptest.NewRecorder()
	dataRequest := httptest.NewRequest(http.MethodGet, "/data/"+create.Key, nil)
	server.ServeHTTP(dataRecorder, dataRequest)

	if status := dataRecorder.Result().StatusCode; status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}

	var data struct {
		Content  string `json:"content"`
		MimeType string `json:"mime_type"`
		IsText   bool   `json:"is_text"`
	}
	if err := json.NewDecoder(dataRecorder.Result().Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode data response: %v", err)
	}

	if data.MimeType != "application/octet-stream" {
		t.Fatalf("expected mime type to round-trip, got %q", data.MimeType)
	}
	if data.IsText {
		t.Fatalf("expected binary payload to be marked as non-text")
	}

	decoded, err := base64.StdEncoding.DecodeString(data.Content)
	if err != nil {
		t.Fatalf("failed to decode content: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Fatalf("expected binary content to match original")
	}
}

func TestCreateTextWithExplicitMIME(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := New(db)

	payload := []byte("hello world")
	reqBody := map[string]any{
		"content":   base64.StdEncoding.EncodeToString(payload),
		"mime_type": "text/plain; charset=utf-8",
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/data", bytes.NewReader(body))
	server.ServeHTTP(recorder, request)

	if status := recorder.Result().StatusCode; status != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, status)
	}

	var create struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(recorder.Result().Body).Decode(&create); err != nil {
		t.Fatalf("failed to decode create response: %v", err)
	}

	dataRecorder := httptest.NewRecorder()
	dataRequest := httptest.NewRequest(http.MethodGet, "/data/"+create.Key, nil)
	server.ServeHTTP(dataRecorder, dataRequest)

	if status := dataRecorder.Result().StatusCode; status != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, status)
	}

	var data struct {
		Content  string `json:"content"`
		MimeType string `json:"mime_type"`
		IsText   bool   `json:"is_text"`
	}
	if err := json.NewDecoder(dataRecorder.Result().Body).Decode(&data); err != nil {
		t.Fatalf("failed to decode data response: %v", err)
	}

	if !data.IsText {
		t.Fatalf("expected text payload to be marked as text")
	}
	if data.MimeType != "text/plain; charset=utf-8" {
		t.Fatalf("expected mime type to round-trip, got %q", data.MimeType)
	}

	decoded, err := base64.StdEncoding.DecodeString(data.Content)
	if err != nil {
		t.Fatalf("failed to decode content: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatalf("expected text content to match original")
	}
}

func newTestDB(t *testing.T) (*ouroboros.OuroborosDB, func()) {
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
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
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
