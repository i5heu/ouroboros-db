package apiServer

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	ouroboros "github.com/i5heu/ouroboros-db"
)

const maxBulkKeys = 500

type bulkDataRequest struct {
	Keys          []string `json:"keys"`
	IncludeBinary bool     `json:"includeBinary,omitempty"`
}

type bulkDataRecord struct {
	Key            string `json:"key"`
	ResolvedKey    string `json:"resolvedKey,omitempty"`
	EditOf         string `json:"editOf,omitempty"`
	Found          bool   `json:"found"`
	MimeType       string `json:"mimeType,omitempty"`
	IsText         bool   `json:"isText,omitempty"`
	SizeBytes      int    `json:"sizeBytes,omitempty"`
	CreatedAt      string `json:"createdAt,omitempty"`
	Title          string `json:"title,omitempty"`
	Content        string `json:"content,omitempty"`
	EncodedContent string `json:"encodedContent,omitempty"`
	Error          string `json:"error,omitempty"`
}

func (s *Server) handleBulkData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req bulkDataRequest
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid request body: %v", err), http.StatusBadRequest)
		return
	}
	if len(req.Keys) == 0 {
		http.Error(w, "keys array is required", http.StatusBadRequest)
		return
	}
	if len(req.Keys) > maxBulkKeys {
		http.Error(w, fmt.Sprintf("too many keys: max %d", maxBulkKeys), http.StatusBadRequest)
		return
	}

	// Deduplicate keys while preserving order.
	uniq := make([]string, 0, len(req.Keys))
	seen := make(map[string]struct{}, len(req.Keys))
	for _, key := range req.Keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		uniq = append(uniq, trimmed)
	}
	if len(uniq) == 0 {
		http.Error(w, "no valid keys supplied", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	writer := bufio.NewWriter(w)

	for _, keyHex := range uniq {
		record := s.fetchBulkRecord(r, keyHex, req.IncludeBinary)
		payload := map[string]any{"type": "record", "record": record}
		if err := encodeNDJSON(writer, payload); err != nil {
			s.log.Error("failed to encode bulk record", "error", err, "key", keyHex)
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

func (s *Server) fetchBulkRecord(r *http.Request, keyHex string, includeBinary bool) bulkDataRecord {
	key, err := cryptHash.HashHexadecimal(keyHex)
	if err != nil {
		return bulkDataRecord{Key: keyHex, Error: fmt.Sprintf("invalid key: %v", err)}
	}

	ctx := r.Context()
	data, err := s.db.GetData(ctx, key)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ouroboros.ErrNotStarted) || errors.Is(err, ouroboros.ErrClosed) {
			status = http.StatusServiceUnavailable
		}
		s.log.Warn("failed to get bulk data", "error", err, "key", keyHex)
		return bulkDataRecord{
			Key:   keyHex,
			Error: http.StatusText(status),
		}
	}

	record := bulkDataRecord{
		Key:       keyHex,
		Found:     true,
		MimeType:  normalizeMime(data.MimeType, data.IsText),
		IsText:    data.IsText,
		SizeBytes: len(data.Content),
		Title:     data.Title,
	}
	resolvedKey := data.ResolvedKey
	if resolvedKey.IsZero() {
		resolvedKey = data.Key
	}
	if resolvedKey != data.Key {
		record.ResolvedKey = resolvedKey.String()
	}
	if !data.EditOf.IsZero() {
		record.EditOf = data.EditOf.String()
	}
	if !data.CreatedAt.IsZero() {
		record.CreatedAt = data.CreatedAt.UTC().Format(time.RFC3339Nano)
	}

	if data.IsText {
		record.Content = string(data.Content)
		return record
	}

	if includeBinary {
		record.EncodedContent = base64.StdEncoding.EncodeToString(data.Content)
	}

	return record
}
