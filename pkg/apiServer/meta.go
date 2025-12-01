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

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
)

const (
	defaultThreadPageSize = 25
	maxTextPayloadBytes   = 256 * 1024
)

type threadSummary struct {
	Key           string `json:"key"`
	Preview       string `json:"preview"`
	Title         string `json:"title,omitempty"`
	MimeType      string `json:"mimeType"`
	IsText        bool   `json:"isText"`
	SizeBytes     int    `json:"sizeBytes"`
	ChildCount    int    `json:"childCount"`
	CreatedAt     string `json:"createdAt,omitempty"`
	ComputedID    string `json:"computedId,omitempty"`
	SuggestedEdit string `json:"suggestedEdit,omitempty"`
}

type threadNode struct {
	Key           string   `json:"key"`
	Parent        string   `json:"parent,omitempty"`
	Title         string   `json:"title,omitempty"`
	MimeType      string   `json:"mimeType"`
	IsText        bool     `json:"isText"`
	SizeBytes     int      `json:"sizeBytes"`
	CreatedAt     string   `json:"createdAt,omitempty"`
	Depth         int      `json:"depth"`
	Children      []string `json:"children"`
	ComputedID    string   `json:"computedId,omitempty"`
	SuggestedEdit string   `json:"suggestedEdit,omitempty"`
	EditOf        string   `json:"editOf,omitempty"`
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

	// Sort by CreatedAt descending (newest first), considering only root thread nodes
	// (nodes without a parent). We need to hydrate metadata for each key in order to
	// determine CreatedAt, root-ness, and eventually stream summaries.
	type keyedMeta struct {
		key       cryptHash.Hash
		createdAt time.Time
	}
	roots := make([]keyedMeta, 0, len(keys))
	for _, k := range keys {
		data, err := s.db.GetData(ctx, k)
		if err != nil {
			s.log.Warn("failed to read metadata for sorting threads", "error", err, "key", k.String())
			continue
		}
		if !data.Parent.IsZero() {
			// Skip entries that already belong to another thread; only emit actual roots
			continue
		}
		roots = append(roots, keyedMeta{key: k, createdAt: data.CreatedAt})
	}

	sort.Slice(roots, func(i, j int) bool {
		if roots[i].createdAt.Equal(roots[j].createdAt) {
			return roots[i].key.String() > roots[j].key.String()
		}
		return roots[i].createdAt.After(roots[j].createdAt)
	})

	limit := parseLimit(r.URL.Query().Get("limit"), defaultThreadPageSize)
	start := decodeCursor(r.URL.Query().Get("cursor"))
	if start < 0 || start >= len(roots) {
		start = 0
	}
	end := start + limit
	if end > len(roots) {
		end = len(roots)
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	encoder := json.NewEncoder(w)

	for idx := start; idx < end; idx++ {
		data, err := s.db.GetData(ctx, roots[idx].key)
		if err != nil {
			s.log.Warn("failed to hydrate thread summary", "error", err, "key", roots[idx].key.String())
			continue
		}
		effective := data
		if !data.SuggestedEdit.IsZero() {
			if latest, latestErr := s.db.GetData(ctx, data.SuggestedEdit); latestErr == nil {
				effective = latest
			} else {
				s.log.Warn("failed to hydrate suggested edit for summary", "error", latestErr, "key", roots[idx].key.String(), "edit", data.SuggestedEdit.String())
			}
		}
		summary := threadSummary{
			Key:        data.Key.String(),
			Title:      effective.Title,
			MimeType:   normalizeMime(effective.MimeType, effective.IsText),
			IsText:     effective.IsText,
			SizeBytes:  len(effective.Content),
			ChildCount: len(data.Children),
		}
		if !effective.CreatedAt.IsZero() {
			summary.CreatedAt = effective.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if data.SuggestedEdit.IsZero() {
			summary.SuggestedEdit = ""
		} else {
			summary.SuggestedEdit = data.SuggestedEdit.String()
		}
		if effective.IsText {
			summary.Preview = clipString(string(effective.Content), 240)
		} else {
			summary.Preview = binaryPreview(summary.MimeType, summary.SizeBytes)
		}
		// Include computed_id if indexer is available
		if s.indexer != nil {
			if cid, err := s.indexer.GetComputedID(roots[idx].key); err == nil && cid != "" {
				summary.ComputedID = cid
			}
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
	if end < len(roots) {
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
		key    cryptHash.Hash
		depth  int
		parent cryptHash.Hash
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

		baseData, err := s.db.GetData(r.Context(), item.key)
		if err != nil {
			s.log.Warn("failed to stream node", "error", err, "key", keyLabel)
			continue
		}

		if !item.parent.IsZero() && !baseData.EditOf.IsZero() && baseData.EditOf == item.parent {
			// Skip explicit edit-reply nodes; the latest edit is surfaced via SuggestedEdit on the parent.
			continue
		}

		nodeData := baseData
		if !baseData.SuggestedEdit.IsZero() {
			if latest, latestErr := s.db.GetData(r.Context(), baseData.SuggestedEdit); latestErr == nil {
				nodeData = latest
			} else {
				s.log.Warn("failed to hydrate suggested edit for node", "error", latestErr, "key", keyLabel, "edit", baseData.SuggestedEdit.String())
			}
		}

		node := threadNode{
			Key:       baseData.Key.String(),
			Title:     nodeData.Title,
			MimeType:  normalizeMime(nodeData.MimeType, nodeData.IsText),
			IsText:    nodeData.IsText,
			SizeBytes: len(nodeData.Content),
			Depth:     item.depth,
			SuggestedEdit: func() string {
				if baseData.SuggestedEdit.IsZero() {
					return ""
				}
				return baseData.SuggestedEdit.String()
			}(),
			EditOf: func() string {
				// Use baseData.EditOf, not nodeData.EditOf, because we want to know
				// if THIS node (by its original key) is an edit, not whether the
				// suggested edit content is an edit of something else.
				if baseData.EditOf.IsZero() {
					return ""
				}
				return baseData.EditOf.String()
			}(),
		}
		if !nodeData.CreatedAt.IsZero() {
			node.CreatedAt = nodeData.CreatedAt.UTC().Format(time.RFC3339Nano)
		}
		if !baseData.Parent.IsZero() {
			node.Parent = baseData.Parent.String()
		}
		// Include computed_id if indexer is available
		if s.indexer != nil {
			if cid, err := s.indexer.GetComputedID(item.key); err == nil && cid != "" {
				node.ComputedID = cid
			}
		}
		node.Children = make([]string, 0, len(baseData.Children))
		for _, child := range baseData.Children {
			if child.IsZero() {
				continue
			}
			node.Children = append(node.Children, child.String())
			if depthLimit < 0 || item.depth+1 <= depthLimit {
				queue = append(queue, queueItem{key: child, depth: item.depth + 1, parent: item.key})
			}
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
