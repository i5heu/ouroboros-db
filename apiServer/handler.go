package apiServer

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/hash"
	ouroboros "github.com/i5heu/ouroboros-db"
)

// TODO: refactor to reduce complexity
func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) { // PA
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

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) { // A
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

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) { // A
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
	if !data.CreatedAt.IsZero() {
		w.Header().Set("X-Ouroboros-Created-At", data.CreatedAt.UTC().Format(time.RFC3339Nano))
	}
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

func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request) { // A
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

	children, err := s.db.ListChildren(r.Context(), key)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ouroboros.ErrNotStarted) || errors.Is(err, ouroboros.ErrClosed) {
			status = http.StatusServiceUnavailable
		}
		s.log.Error("failed to list children", "error", err, "key", keyHex)
		http.Error(w, http.StatusText(status), status)
		return
	}

	response := listResponse{Keys: make([]string, 0, len(children))}
	for _, child := range children {
		if child.IsZero() {
			continue
		}
		response.Keys = append(response.Keys, child.String())
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleAuthProcess(w http.ResponseWriter, r *http.Request) { // A
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // limit to 1MB
	dec.DisallowUnknownFields()

	var req authProcessReq

	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	s.authProcess(r.Context(), w, req)
}
