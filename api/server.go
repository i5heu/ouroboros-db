package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	cryptHash "github.com/i5heu/ouroboros-crypt/hash"
	ouroboros "github.com/i5heu/ouroboros-db"
)

const (
	defaultDataShards   = 4
	defaultParityShards = 2
)

type Server struct {
	mux  *http.ServeMux
	db   *ouroboros.OuroborosDB
	log  *slog.Logger
	auth AuthFunc
}

type Option func(*Server)

func WithLogger(logger *slog.Logger) Option {
	return func(s *Server) {
		if logger != nil {
			s.log = logger
		}
	}
}

type AuthFunc func(*http.Request) error

func WithAuth(auth AuthFunc) Option {
	return func(s *Server) {
		if auth != nil {
			s.auth = auth
		}
	}
}

func defaultAuth(*http.Request) error {
	return nil
}

func New(db *ouroboros.OuroborosDB, opts ...Option) *Server {
	s := &Server{
		mux:  http.NewServeMux(),
		db:   db,
		log:  slog.Default(),
		auth: defaultAuth,
	}

	for _, opt := range opts {
		opt(s)
	}

	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /data", s.handleCreate)
	s.mux.HandleFunc("GET /data/{key}", s.handleGet)
	s.mux.HandleFunc("GET /data", s.handleList)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	origin := r.Header.Get("Origin")
	if origin == "" {
		origin = "*"
	} else {
		w.Header().Set("Vary", "Origin")
	}

	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	}

	allowedHeaders := r.Header.Get("Access-Control-Request-Headers")
	if allowedHeaders == "" {
		allowedHeaders = "Content-Type, Accept"
	}
	w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
	w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
	w.Header().Set(
		"Access-Control-Expose-Headers",
		"Content-Type, Content-Length, X-Ouroboros-Key, X-Ouroboros-Mime, X-Ouroboros-Is-Text, X-Ouroboros-Parent, X-Ouroboros-Children",
	)

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := s.auth(r); err != nil {
		s.log.Warn("authentication failed", "error", err)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}

	s.mux.ServeHTTP(w, r)
}

type createResponse struct {
	Key string `json:"key"`
}

type listResponse struct {
	Keys []string `json:"keys"`
}

type createMetadata struct {
	ReedSolomonShards       uint8    `json:"reed_solomon_shards"`
	ReedSolomonParityShards uint8    `json:"reed_solomon_parity_shards"`
	Parent                  string   `json:"parent"`
	Children                []string `json:"children"`
	MimeType                string   `json:"mime_type"`
	IsText                  *bool    `json:"is_text"`
	Filename                string   `json:"filename"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "multipart/form-data") {
		http.Error(w, "expected multipart/form-data", http.StatusUnsupportedMediaType)
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, fmt.Sprintf("failed to parse multipart form: %v", err), http.StatusBadRequest)
		return
	}

	file, fileHeader, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field is required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	payload, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read file: %v", err), http.StatusBadRequest)
		return
	}

	metaValue := r.FormValue("metadata")
	var meta createMetadata
	if strings.TrimSpace(metaValue) != "" {
		if err := json.Unmarshal([]byte(metaValue), &meta); err != nil {
			http.Error(w, fmt.Sprintf("invalid metadata: %v", err), http.StatusBadRequest)
			return
		}
	}

	parentHash, err := parseHash(meta.Parent)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid parent hash: %v", err), http.StatusBadRequest)
		return
	}

	childrenHashes := make([]cryptHash.Hash, 0, len(meta.Children))
	for _, child := range meta.Children {
		h, err := parseHash(child)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid child hash: %v", err), http.StatusBadRequest)
			return
		}
		if !h.IsZero() {
			childrenHashes = append(childrenHashes, h)
		}
	}

	shards := meta.ReedSolomonShards
	if shards == 0 {
		shards = defaultDataShards
	}
	parity := meta.ReedSolomonParityShards
	if parity == 0 {
		parity = defaultParityShards
	}

	mimeType := strings.TrimSpace(meta.MimeType)
	if mimeType == "" {
		mimeType = strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	}
	if meta.IsText != nil && *meta.IsText && mimeType == "" {
		mimeType = "text/plain; charset=utf-8"
	}

	key, err := s.db.StoreData(r.Context(), payload, ouroboros.StoreOptions{
		Parent:                  parentHash,
		Children:                childrenHashes,
		ReedSolomonShards:       shards,
		ReedSolomonParityShards: parity,
		MimeType:                mimeType,
	})
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ouroboros.ErrNotStarted) || errors.Is(err, ouroboros.ErrClosed) {
			status = http.StatusServiceUnavailable
		}
		s.log.Error("failed to store data", "error", err)
		http.Error(w, http.StatusText(status), status)
		return
	}

	writeJSON(w, http.StatusCreated, createResponse{Key: key.String()})
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	keys, err := s.db.ListData(r.Context())
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ouroboros.ErrNotStarted) || errors.Is(err, ouroboros.ErrClosed) {
			status = http.StatusServiceUnavailable
		}
		s.log.Error("failed to list data", "error", err)
		http.Error(w, http.StatusText(status), status)
		return
	}

	response := listResponse{Keys: make([]string, 0, len(keys))}
	for _, key := range keys {
		response.Keys = append(response.Keys, key.String())
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	keyHex := r.PathValue("key")
	if keyHex == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	key, err := cryptHash.HashHexadecimal(keyHex)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid key: %v", err), http.StatusBadRequest)
		return
	}

	data, err := s.db.GetData(r.Context(), key)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ouroboros.ErrNotStarted) || errors.Is(err, ouroboros.ErrClosed) {
			status = http.StatusServiceUnavailable
		}
		s.log.Error("failed to get data", "error", err, "key", keyHex)
		http.Error(w, http.StatusText(status), status)
		return
	}

	mimeType := strings.TrimSpace(data.MimeType)
	if mimeType == "" {
		if data.IsText {
			mimeType = "text/plain; charset=utf-8"
		} else {
			mimeType = "application/octet-stream"
		}
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data.Content)))
	w.Header().Set("X-Ouroboros-Key", keyHex)
	w.Header().Set("X-Ouroboros-Is-Text", strconv.FormatBool(data.IsText))
	w.Header().Set("X-Ouroboros-Mime", mimeType)
	if !data.Parent.IsZero() {
		w.Header().Set("X-Ouroboros-Parent", data.Parent.String())
	}
	if len(data.Children) > 0 {
		children := make([]string, 0, len(data.Children))
		for _, child := range data.Children {
			if child.IsZero() {
				continue
			}
			children = append(children, child.String())
		}
		if len(children) > 0 {
			w.Header().Set("X-Ouroboros-Children", strings.Join(children, ","))
		}
	}

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data.Content); err != nil {
		s.log.Error("failed to write response body", "error", err, "key", keyHex)
	}
}

func parseHash(value string) (cryptHash.Hash, error) {
	if value == "" {
		return cryptHash.Hash{}, nil
	}

	return cryptHash.HashHexadecimal(value)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Default().Error("failed to encode response", "error", err)
	}
}
