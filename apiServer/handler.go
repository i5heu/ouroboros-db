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

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
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
	w.Header().Set("Accept-Ranges", "bytes")

	status := http.StatusOK
	responseBody := data.Content
	totalSize := len(data.Content)
	if rangeHeader := strings.TrimSpace(r.Header.Get("Range")); rangeHeader != "" {
		start, end, err := parseByteRange(rangeHeader, totalSize)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
			http.Error(w, http.StatusText(http.StatusRequestedRangeNotSatisfiable), http.StatusRequestedRangeNotSatisfiable)
			return
		}
		responseBody = data.Content[start : end+1]
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
		status = http.StatusPartialContent
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(responseBody)))

	w.WriteHeader(status)
	if _, err := w.Write(responseBody); err != nil {
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

func (s *Server) handleAuthProcess(w http.ResponseWriter, r *http.Request) { // AC
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req authProcessReq
	useBody := r.Body != nil && r.Body != http.NoBody && (r.ContentLength > 0 || r.Method == http.MethodPost)

	if useBody {
		dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20)) // limit to 1MB
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		query := r.URL.Query()
		req.EncryptedData = query.Get("data")
		req.SHA512 = query.Get("sha512")
	}

	s.authProcess(r.Context(), w, req)
}

func parseByteRange(header string, size int) (int, int, error) {
	if size <= 0 {
		return 0, 0, fmt.Errorf("invalid size for range")
	}
	if header == "" {
		return 0, size - 1, nil
	}
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, fmt.Errorf("unsupported range unit")
	}
	value := strings.TrimSpace(header[len("bytes="):])
	parts := strings.Split(value, ",")
	if len(parts) != 1 {
		return 0, 0, fmt.Errorf("multiple ranges not supported")
	}
	rangeSpec := strings.TrimSpace(parts[0])
	rangeParts := strings.SplitN(rangeSpec, "-", 2)
	if len(rangeParts) != 2 {
		return 0, 0, fmt.Errorf("malformed range")
	}
	startStr := strings.TrimSpace(rangeParts[0])
	endStr := strings.TrimSpace(rangeParts[1])

	switch {
	case startStr == "" && endStr == "":
		return 0, 0, fmt.Errorf("empty range")
	case startStr == "":
		suffix, err := strconv.Atoi(endStr)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("invalid suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	case endStr == "":
		start, err := strconv.Atoi(startStr)
		if err != nil || start < 0 || start >= size {
			return 0, 0, fmt.Errorf("invalid start range")
		}
		return start, size - 1, nil
	default:
		start, err := strconv.Atoi(startStr)
		if err != nil || start < 0 {
			return 0, 0, fmt.Errorf("invalid start value")
		}
		end, err := strconv.Atoi(endStr)
		if err != nil || end < start {
			return 0, 0, fmt.Errorf("invalid end value")
		}
		if end >= size {
			end = size - 1
		}
		return start, end, nil
	}
}
