package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

type createRequest struct {
	Content                 string   `json:"content"`
	ReedSolomonShards       uint8    `json:"reed_solomon_shards"`
	ReedSolomonParityShards uint8    `json:"reed_solomon_parity_shards"`
	Parent                  string   `json:"parent"`
	Children                []string `json:"children"`
	MimeType                string   `json:"mime_type"`
}

type createResponse struct {
	Key string `json:"key"`
}

type listResponse struct {
	Keys []string `json:"keys"`
}

type dataResponse struct {
	Key      string   `json:"key"`
	Content  string   `json:"content"`
	MimeType string   `json:"mime_type,omitempty"`
	IsText   bool     `json:"is_text"`
	Parent   string   `json:"parent,omitempty"`
	Children []string `json:"children,omitempty"`
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20))
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read body: %v", err), http.StatusBadRequest)
		return
	}

	var req createRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, fmt.Sprintf("invalid json: %v", err), http.StatusBadRequest)
		return
	}

	if req.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	payload, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		http.Error(w, "content must be base64 encoded", http.StatusBadRequest)
		return
	}

	parentHash, err := parseHash(req.Parent)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid parent hash: %v", err), http.StatusBadRequest)
		return
	}

	childrenHashes := make([]cryptHash.Hash, 0, len(req.Children))
	for _, child := range req.Children {
		h, err := parseHash(child)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid child hash: %v", err), http.StatusBadRequest)
			return
		}
		if !h.IsZero() {
			childrenHashes = append(childrenHashes, h)
		}
	}

	shards := req.ReedSolomonShards
	if shards == 0 {
		shards = defaultDataShards
	}
	parity := req.ReedSolomonParityShards
	if parity == 0 {
		parity = defaultParityShards
	}

	mimeType := strings.TrimSpace(req.MimeType)

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

	response := dataResponse{
		Key:     keyHex,
		Content: base64.StdEncoding.EncodeToString(data.Content),
		IsText:  data.IsText,
	}
	if data.MimeType != "" {
		response.MimeType = data.MimeType
	}
	if !data.Parent.IsZero() {
		response.Parent = data.Parent.String()
	}
	if len(data.Children) > 0 {
		for _, child := range data.Children {
			if child.IsZero() {
				continue
			}
			response.Children = append(response.Children, child.String())
		}
	}

	writeJSON(w, http.StatusOK, response)
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
