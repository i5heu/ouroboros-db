package apiServer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	cryptHash "github.com/i5heu/ouroboros-crypt/hash"
)

const (
	defaultThreadPageSize = 25
	maxTextPayloadBytes   = 256 * 1024
)

type threadSummary struct {
	Key        string `json:"key"`
	Preview    string `json:"preview"`
	MimeType   string `json:"mimeType"`
	IsText     bool   `json:"isText"`
	SizeBytes  int    `json:"sizeBytes"`
	ChildCount int    `json:"childCount"`
	CreatedAt  string `json:"createdAt,omitempty"`
}

type threadNode struct {
	Key       string   `json:"key"`
	Parent    string   `json:"parent,omitempty"`
	MimeType  string   `json:"mimeType"`
	IsText    bool     `json:"isText"`
	SizeBytes int      `json:"sizeBytes"`
	CreatedAt string   `json:"createdAt,omitempty"`
	Depth     int      `json:"depth"`
	Children  []string `json:"children"`
	Content   string   `json:"content,omitempty"`
	Preview   string   `json:"preview,omitempty"`
}

func (s *Server) handleThreadSummaries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	keys, err := s.db.ListData(ctx)
	if err != nil {
		s.log.Error("failed to list thread keys", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	// Sort by CreatedAt descending (newest first).
	// We need to hydrate metadata for each key in order to determine CreatedAt.
	// Fall back to zero time for any keys that fail to hydrate so they'll sort last.
	type keyedMeta struct {
		key       cryptHash.Hash
		createdAt time.Time
	}
	metas := make([]keyedMeta, 0, len(keys))
	for _, k := range keys {
		var created time.Time
		if data, err := s.db.GetData(ctx, k); err == nil {
			created = data.CreatedAt
		} else {
			s.log.Warn("failed to read metadata for sorting threads", "error", err, "key", k.String())
		}
		metas = append(metas, keyedMeta{key: k, createdAt: created})
	}

	sort.Slice(metas, func(i, j int) bool {
		if metas[i].createdAt.Equal(metas[j].createdAt) {
			// deterministic fallback: sort by key string descending so newly created keys
			// with equal timestamps still have a stable order.
			return metas[i].key.String() > metas[j].key.String()
		}
		return metas[i].createdAt.After(metas[j].createdAt)
	})

	// Rebuild keys in sorted order.
	for i := range metas {
		keys[i] = metas[i].key
	}

	limit := parseLimit(r.URL.Query().Get("limit"), defaultThreadPageSize)
	start := decodeCursor(r.URL.Query().Get("cursor"))
	if start < 0 || start >= len(keys) {
		start = 0
	}
	end := start + limit
	if end > len(keys) {
		end = len(keys)
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)

	for idx := start; idx < end; idx++ {
		data, err := s.db.GetData(ctx, keys[idx])
		if err != nil {
			s.log.Warn("failed to hydrate thread summary", "error", err, "key", keys[idx].String())
			continue
		}
		summary := threadSummary{
			Key:        data.Key.String(),
			MimeType:   normalizeMime(data.MimeType, data.IsText),
			IsText:     data.IsText,
			SizeBytes:  len(data.Content),
			ChildCount: len(data.Children),
		}
		if !data.CreatedAt.IsZero() {
			summary.CreatedAt = data.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if data.IsText {
			summary.Preview = clipString(string(data.Content), 240)
		} else {
			summary.Preview = binaryPreview(summary.MimeType, summary.SizeBytes)
		}

		env := map[string]any{"type": "thread", "thread": summary}
		if err := encoder.Encode(env); err != nil {
			s.log.Error("failed to stream thread summary", "error", err)
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
	}

	nextCursor := ""
	if end < len(keys) {
		nextCursor = strconv.Itoa(end)
	}
	cursorPayload := map[string]any{"type": "cursor", "cursor": nextCursor}
	_ = encoder.Encode(cursorPayload)
}

func (s *Server) handleThreadNodeStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	keyHex := strings.TrimSpace(r.PathValue("key"))
	if keyHex == "" {
		http.Error(w, "missing key", http.StatusBadRequest)
		return
	}

	rootKey, err := cryptHash.HashHexadecimal(keyHex)
	if err != nil {
		http.Error(w, fmt.Sprintf("invalid key: %v", err), http.StatusBadRequest)
		return
	}

	maxDepth := r.URL.Query().Get("depth")
	depthLimit := -1
	if maxDepth != "" {
		if parsed, convErr := strconv.Atoi(maxDepth); convErr == nil && parsed >= 0 {
			depthLimit = parsed
		}
	}

	type queueItem struct {
		key   cryptHash.Hash
		depth int
	}

	queue := []queueItem{{key: rootKey, depth: 0}}
	seen := make(map[string]struct{})

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	writer := bufio.NewWriter(w)

	for len(queue) > 0 {
		if ctxErr := r.Context().Err(); ctxErr != nil {
			break
		}
		item := queue[0]
		queue = queue[1:]

		keyLabel := item.key.String()
		if _, exists := seen[keyLabel]; exists {
			continue
		}
		seen[keyLabel] = struct{}{}

		data, err := s.db.GetData(r.Context(), item.key)
		if err != nil {
			s.log.Warn("failed to stream node", "error", err, "key", keyLabel)
			continue
		}

		node := threadNode{
			Key:       data.Key.String(),
			MimeType:  normalizeMime(data.MimeType, data.IsText),
			IsText:    data.IsText,
			SizeBytes: len(data.Content),
			Depth:     item.depth,
		}
		if !data.CreatedAt.IsZero() {
			node.CreatedAt = data.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if !data.Parent.IsZero() {
			node.Parent = data.Parent.String()
		}
		node.Children = make([]string, 0, len(data.Children))
		for _, child := range data.Children {
			if child.IsZero() {
				continue
			}
			node.Children = append(node.Children, child.String())
			if depthLimit < 0 || item.depth+1 <= depthLimit {
				queue = append(queue, queueItem{key: child, depth: item.depth + 1})
			}
		}

		if data.IsText {
			node.Content = clipString(string(data.Content), maxTextPayloadBytes)
		} else {
			node.Preview = binaryPreview(node.MimeType, node.SizeBytes)
		}

		payload := map[string]any{"type": "node", "node": node}
		if err := encodeNDJSON(writer, payload); err != nil {
			s.log.Error("failed to encode node payload", "error", err)
			return
		}
		if flusher != nil {
			writer.Flush()
			flusher.Flush()
		}
	}

	_ = encodeNDJSON(writer, map[string]any{"type": "eof"})
	writer.Flush()
}

func encodeNDJSON(w *bufio.Writer, payload any) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := w.Write(bytes); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return nil
}

func parseLimit(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > 500 {
		return 500
	}
	return value
}

func decodeCursor(raw string) int {
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func normalizeMime(mime string, isText bool) string {
	trimmed := strings.TrimSpace(mime)
	if trimmed != "" {
		return strings.ToLower(trimmed)
	}
	if isText {
		return "text/plain; charset=utf-8"
	}
	return "application/octet-stream"
}

func clipString(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(value) <= limit && utf8.ValidString(value) {
		return strings.TrimSpace(value)
	}

	clipped := value
	if len(clipped) > limit {
		clipped = clipped[:limit]
	}
	for !utf8.ValidString(clipped) && len(clipped) > 0 {
		clipped = clipped[:len(clipped)-1]
	}
	clipped = strings.TrimSpace(clipped)
	if clipped == "" {
		return ""
	}
	return clipped + "…"
}

func binaryPreview(mime string, size int) string {
	return fmt.Sprintf("[%s • %s]", mime, formatBytes(size))
}

func formatBytes(size int) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	const unit = 1024.0
	value := float64(size)
	units := []string{"KB", "MB", "GB", "TB"}
	idx := 0
	for value >= unit && idx < len(units)-1 {
		value /= unit
		idx++
	}
	if value >= 10 {
		return fmt.Sprintf("%.0f %s", value, units[idx])
	}
	return fmt.Sprintf("%.1f %s", value, units[idx])
}
