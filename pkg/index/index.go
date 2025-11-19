package index

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"

	"github.com/blevesearch/bleve/v2"

	hash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/encoding"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

//TODO : implement thread root to last child edit timestamp to optimize sorting by last activity
//TODO: implement text search via bleve full text search index
//TODO: implement semantic text search with bleve vector search and Qwen/Qwen3-Embedding-0.6B embeddings with a API or something like that and we need to handle different models
//TODO: check if it would be better to have a bleve on disk instead of in memory only

type Indexer struct {
	log *slog.Logger

	kv atomic.Pointer[ouroboroskv.KV]
	bi bleve.Index
}

func NewIndexer(kv *ouroboroskv.KV, logger *slog.Logger) *Indexer { //A
	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		panic(err)
	}

	_ = index
	idx := &Indexer{
		log: logger,
		bi:  index,
		kv:  atomic.Pointer[ouroboroskv.KV]{},
	}
	// Store kv pointer for the indexer to be able to fetch data for indexing
	if kv != nil {
		idx.kv.Store(kv)
	}
	return idx
}

func (idx *Indexer) Close() error {
	return idx.bi.Close()
}

// ReindexAll will walk the KV store and index all present records. This is a convenience
// initial population for the in-memory index. It returns an error on failure to list roots
// or read data from the store; individual record failures are logged and indexing continues.
func (idx *Indexer) ReindexAll() error { //A
	kv := idx.kv.Load()
	if kv == nil {
		return fmt.Errorf("kv handle not available")
	}

	keys, err := kv.ListRootKeys()
	if err != nil {
		return fmt.Errorf("list root keys: %w", err)
	}

	visited := make(map[string]struct{})
	queue := make([]hash.Hash, 0, len(keys))
	queue = append(queue, keys...)
	for len(queue) > 0 {
		k := queue[0]
		queue = queue[1:]
		keyStr := k.String()
		if _, ok := visited[keyStr]; ok {
			continue
		}
		visited[keyStr] = struct{}{}

		// Index the item; continue on error
		if err := idx.IndexHash(k); err != nil {
			// don't fail the entire reindex on single record error; log and continue
			// but index package should not import slog/log to avoid cycles.
			// For now, continue.
			idx.log.Error("reindex: index hash failed", "key", keyStr, "error", err)
		}

		children, err := kv.GetChildren(k)
		if err != nil {
			continue
		}
		for _, c := range children {
			if c.IsZero() {
				continue
			}
			if _, ok := visited[c.String()]; !ok {
				queue = append(queue, c)
			}
		}
	}
	return nil
}

// We will not implement ReindexAll for now
// func (idx *Indexer) ReindexAll() error {
// 	// this needs to be done in batches and in a separate goroutine
// 	return nil
// }

func (idx *Indexer) IndexHash(cr hash.Hash) error { //A
	if idx == nil {
		return fmt.Errorf("indexer is nil")
	}
	kv := idx.kv.Load()
	if kv == nil {
		return fmt.Errorf("kv handle not available")
	}

	// Read raw data from KV
	data, err := kv.ReadData(cr)
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	// Decode content and optional mime header
	content, payloadHeader, isMime, err := encoding.DecodeContent(data.Content)
	if err != nil {
		return fmt.Errorf("decode content: %w", err)
	}
	mimeType := ""
	isText := false
	if isMime && len(payloadHeader) > 0 {
		mimeType = string(payloadHeader)
		if strings.HasPrefix(mimeType, "text/") {
			isText = true
		}
	}

	// Parse metadata for createdAt if present
	var createdAt int64
	if len(data.MetaData) > 0 {
		var meta struct {
			CreatedAt string `json:"created_at"`
		}
		if err := json.Unmarshal(data.MetaData, &meta); err == nil && meta.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, meta.CreatedAt); err == nil {
				createdAt = t.UnixNano()
			}
		}
	}

	doc := map[string]any{
		"key":        cr.String(),
		"content":    string(content),
		"mimeType":   mimeType,
		"isText":     isText,
		"createdAt":  createdAt,
		"childCount": len(data.Children),
		"parent":     data.Parent.String(),
	}

	if !isText {
		// If not a text payload, we may still want to index the metadata, but we don't
		// want to populate the content with binary data.
		doc["content"] = ""
	}

	return idx.bi.Index(cr.String(), doc)
}

func (idx *Indexer) RemoveHash(cr hash.Hash) error { //A
	if idx == nil {
		return fmt.Errorf("indexer is nil")
	}
	return idx.bi.Delete(cr.String())
}

func (idx *Indexer) TextSearch(query string, limit int) ([]hash.Hash, error) { //A
	if idx == nil {
		return nil, fmt.Errorf("indexer is nil")
	}
	if idx.bi == nil {
		return nil, fmt.Errorf("bleve index not initialized")
	}
	if limit <= 0 {
		limit = 25
	}

	q := bleve.NewQueryStringQuery(query)
	search := bleve.NewSearchRequestOptions(q, limit, 0, false)
	// Sort by relevance (default), caller can override if needed
	search.Fields = []string{"key"}

	res, err := idx.bi.Search(search)
	if err != nil {
		return nil, err
	}

	out := make([]hash.Hash, 0, len(res.Hits))
	for _, hit := range res.Hits {
		if hit == nil || hit.ID == "" {
			continue
		}
		h, err := hash.HashHexadecimal(hit.ID)
		if err != nil {
			// skip invalid ids
			continue
		}
		out = append(out, h)
	}
	return out, nil
}

func (idx *Indexer) LastChildActivity(hash hash.Hash) (int64, error) { //A
	if idx == nil {
		return 0, fmt.Errorf("indexer is nil")
	}
	kv := idx.kv.Load()
	if kv == nil {
		return 0, fmt.Errorf("kv handle not available")
	}
	children, err := kv.GetChildren(hash)
	if err != nil {
		return 0, fmt.Errorf("get children: %w", err)
	}
	var last int64
	for _, child := range children {
		data, err := kv.ReadData(child)
		if err != nil {
			continue
		}
		if len(data.MetaData) == 0 {
			continue
		}
		var meta struct {
			CreatedAt string `json:"created_at"`
		}
		if err := json.Unmarshal(data.MetaData, &meta); err != nil {
			continue
		}
		if meta.CreatedAt == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, meta.CreatedAt)
		if err != nil {
			continue
		}
		if t.UnixNano() > last {
			last = t.UnixNano()
		}
	}
	return last, nil
}
