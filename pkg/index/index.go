package index

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/token/edgengram"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode"
	"github.com/blevesearch/bleve/v2/mapping"

	hash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/meta"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

//TODO : implement thread root to last child edit timestamp to optimize sorting by last activity
//TODO: implement text search via bleve full text search index
//TODO: implement semantic text search with bleve vector search and Qwen/Qwen3-Embedding-0.6B embeddings with a API or something like that and we need to handle different models
//TODO: check if it would be better to have a bleve on disk instead of in memory only

type Indexer struct {
	log *slog.Logger

	kv ouroboroskv.Store
	bi bleve.Index

	// computedIDIndex maps computed_id (e.g., "thoughts:gravitation:physics") to a list of hashes.
	// Multiple messages can share the same computed_id.
	computedIDMu    sync.RWMutex
	computedIDIndex map[string][]hash.Hash

	// hashToComputedID maps hash string to its computed_id for quick lookups
	hashToComputedID map[string]string

	// latestEdits maps a base hash (string) to the most recent edit hash and its timestamp.
	editMu      sync.RWMutex
	latestEdits map[string]editEntry
}

type editEntry struct {
	hash      hash.Hash
	createdAt int64
}

const (
	contentAnalyzerName    = "contentEdgeNgram"
	contentTokenFilterName = "contentEdgeFilter"
)

func buildIndexMapping() (mapping.IndexMapping, error) { //A
	defaultMapping := bleve.NewDocumentMapping()
	contentField := bleve.NewTextFieldMapping()
	contentField.Analyzer = contentAnalyzerName
	defaultMapping.AddFieldMappingsAt("content", contentField)

	idxMapping := bleve.NewIndexMapping()
	idxMapping.DefaultMapping = defaultMapping
	idxMapping.DefaultAnalyzer = contentAnalyzerName

	if err := idxMapping.AddCustomTokenFilter(contentTokenFilterName, map[string]any{
		"type": edgengram.Name,
		"min":  3.0,
		"max":  25.0,
	}); err != nil {
		return nil, fmt.Errorf("add token filter: %w", err)
	}

	if err := idxMapping.AddCustomAnalyzer(contentAnalyzerName, map[string]any{
		"type":      custom.Name,
		"tokenizer": unicode.Name,
		"token_filters": []string{
			lowercase.Name,
			contentTokenFilterName,
		},
	}); err != nil {
		return nil, fmt.Errorf("add analyzer: %w", err)
	}

	return idxMapping, nil
}

func NewIndexer(kv ouroboroskv.Store, logger *slog.Logger) *Indexer { //A
	mapping, err := buildIndexMapping()
	if err != nil {
		panic(err)
	}

	index, err := bleve.NewMemOnly(mapping)
	if err != nil {
		panic(err)
	}

	idx := &Indexer{
		log:              logger,
		bi:               index,
		kv:               kv,
		computedIDIndex:  make(map[string][]hash.Hash),
		hashToComputedID: make(map[string]string),
		latestEdits:      make(map[string]editEntry),
	}
	return idx
}

func (idx *Indexer) Close() error { //AC
	return idx.bi.Close()
}

// ReindexAll will walk the KV store and index all present records. This is a convenience
// initial population for the in-memory index. It returns an error on failure to list roots
// or read data from the store; individual record failures are logged and indexing continues.
func (idx *Indexer) ReindexAll() error { //A
	kv := idx.kv
	if kv == nil {
		return fmt.Errorf("kv handle not available")
	}

	// Reset in-memory indices before rebuilding.
	idx.computedIDMu.Lock()
	idx.computedIDIndex = make(map[string][]hash.Hash)
	idx.hashToComputedID = make(map[string]string)
	idx.computedIDMu.Unlock()
	idx.editMu.Lock()
	idx.latestEdits = make(map[string]editEntry)
	idx.editMu.Unlock()

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

	idx.log.Info("reindex: completed", "total_indexed", len(visited))
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
	kv := idx.kv
	if kv == nil {
		return fmt.Errorf("kv handle not available")
	}

	// Read raw data from KV
	data, err := kv.ReadData(cr)
	if err != nil {
		return fmt.Errorf("read data: %w", err)
	}

	// Content is stored raw; metadata stores MIME type.
	content := data.Content
	mimeType := ""
	isText := false

	// Parse metadata for createdAt, title and mime type if present
	var createdAt int64
	var title string
	var editOf hash.Hash
	if len(data.Meta) > 0 {
		var md meta.Metadata
		if err := json.Unmarshal(data.Meta, &md); err == nil {
			if !md.CreatedAt.IsZero() {
				createdAt = md.CreatedAt.UnixNano()
			}
			title = md.Title
			if md.MimeType != "" {
				mimeType = md.MimeType
				if strings.HasPrefix(mimeType, "text/") {
					isText = true
				}
			}
			if strings.TrimSpace(md.EditOf) != "" {
				if parsed, err := hash.HashHexadecimal(md.EditOf); err == nil {
					editOf = parsed
				}
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
		"title":      title,
	}

	if !isText {
		// If not a text payload, we may still want to index the metadata, but we don't
		// want to populate the content with binary data.
		doc["content"] = ""
	}

	// Compute and store the computed_id based on ancestor titles
	computedID := idx.computeIDForHash(cr, title)
	if computedID != "" {
		doc["computedId"] = computedID
		idx.storeComputedID(computedID, cr)
	}

	// Track latest edit mapping if this record edits another hash
	if !editOf.IsZero() {
		idx.storeLatestEdit(editOf, cr, createdAt)
		doc["editOf"] = editOf.String()
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

	match := bleve.NewMatchQuery(query)
	match.Analyzer = contentAnalyzerName
	search := bleve.NewSearchRequestOptions(match, limit, 0, false)
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
	kv := idx.kv
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
		if len(data.Meta) == 0 {
			continue
		}
		var md meta.Metadata
		if err := json.Unmarshal(data.Meta, &md); err != nil {
			continue
		}
		if md.CreatedAt.IsZero() {
			continue
		}
		t := md.CreatedAt
		// md.CreatedAt is a time.Time parsed by Unmarshal; if it's zero we'd have already continued.
		if t.UnixNano() > last {
			last = t.UnixNano()
		}
	}
	return last, nil
}

// computeIDForHash builds a computed_id by walking up the parent chain and
// joining titles with ":". The format is "root_title:parent_title:...:this_title".
// If a message has no title, its hash prefix is used instead.
func (idx *Indexer) computeIDForHash(h hash.Hash, currentTitle string) string {
	if idx == nil || idx.kv == nil {
		return ""
	}

	// Collect ancestor titles by walking up the parent chain
	var titles []string
	current := h

	// First, add the current message's title
	if currentTitle != "" {
		titles = append(titles, sanitizeIDSegment(currentTitle))
	} else {
		// Use a short hash prefix as fallback
		titles = append(titles, h.String()[:8])
	}

	// Walk up the parent chain
	for {
		parent, err := idx.kv.GetParent(current)
		if err != nil || parent.IsZero() {
			break
		}

		// Read parent metadata to get its title
		data, err := idx.kv.ReadData(parent)
		if err != nil {
			break
		}

		parentTitle := ""
		if len(data.Meta) > 0 {
			var md meta.Metadata
			if err := json.Unmarshal(data.Meta, &md); err == nil {
				parentTitle = md.Title
			}
		}

		if parentTitle != "" {
			titles = append(titles, sanitizeIDSegment(parentTitle))
		} else {
			titles = append(titles, parent.String()[:8])
		}

		current = parent
	}

	// Reverse the titles so root comes first
	for i, j := 0, len(titles)-1; i < j; i, j = i+1, j-1 {
		titles[i], titles[j] = titles[j], titles[i]
	}

	return strings.Join(titles, ":")
}

// sanitizeIDSegment cleans a title for use in a computed_id by:
// - Converting umlauts to ASCII equivalents (ä→ae, ö→oe, ü→ue, ß→ss, etc.)
// - Replacing spaces and special characters with underscores
// - Keeping only alphanumeric characters and underscores
func sanitizeIDSegment(s string) string {
	s = strings.TrimSpace(s)

	// First, convert umlauts and special letters to ASCII equivalents
	replacements := map[rune]string{
		'ä': "ae", 'Ä': "Ae",
		'ö': "oe", 'Ö': "Oe",
		'ü': "ue", 'Ü': "Ue",
		'ß': "ss",
		'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'å': "a", 'À': "A", 'Á': "A", 'Â': "A", 'Ã': "A", 'Å': "A",
		'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'È': "E", 'É': "E", 'Ê': "E", 'Ë': "E",
		'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'Ì': "I", 'Í': "I", 'Î': "I", 'Ï': "I",
		'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ø': "o", 'Ò': "O", 'Ó': "O", 'Ô': "O", 'Õ': "O", 'Ø': "O",
		'ù': "u", 'ú': "u", 'û': "u", 'Ù': "U", 'Ú': "U", 'Û': "U",
		'ý': "y", 'ÿ': "y", 'Ý': "Y",
		'ñ': "n", 'Ñ': "N",
		'ç': "c", 'Ç': "C",
		'æ': "ae", 'Æ': "Ae",
		'œ': "oe", 'Œ': "Oe",
	}

	var result strings.Builder
	result.Grow(len(s) * 2) // Pre-allocate for potential expansions

	for _, r := range s {
		if replacement, ok := replacements[r]; ok {
			result.WriteString(replacement)
		} else if isAlphanumeric(r) {
			result.WriteRune(r)
		} else {
			// Replace spaces and any other special character with underscore
			result.WriteRune('_')
		}
	}

	s = result.String()

	// Collapse multiple underscores
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}

	// Trim leading/trailing underscores
	s = strings.Trim(s, "_")

	return s
}

// isAlphanumeric returns true if the rune is a-z, A-Z, or 0-9
func isAlphanumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// storeComputedID adds a hash to the computed_id index.
func (idx *Indexer) storeComputedID(computedID string, h hash.Hash) {
	if idx == nil || computedID == "" {
		return
	}

	idx.computedIDMu.Lock()
	defer idx.computedIDMu.Unlock()

	hashStr := h.String()

	// Check if this hash already has a computed_id and remove it from old mapping
	if oldID, exists := idx.hashToComputedID[hashStr]; exists && oldID != computedID {
		// Remove from old computed_id list
		oldList := idx.computedIDIndex[oldID]
		newList := make([]hash.Hash, 0, len(oldList))
		for _, existing := range oldList {
			if existing.String() != hashStr {
				newList = append(newList, existing)
			}
		}
		if len(newList) > 0 {
			idx.computedIDIndex[oldID] = newList
		} else {
			delete(idx.computedIDIndex, oldID)
		}
	}

	// Add to new computed_id list if not already present
	list := idx.computedIDIndex[computedID]
	found := false
	for _, existing := range list {
		if existing.String() == hashStr {
			found = true
			break
		}
	}
	if !found {
		idx.computedIDIndex[computedID] = append(list, h)
	}

	idx.hashToComputedID[hashStr] = computedID
}

// storeLatestEdit records the newest edit for a given base hash based on createdAt.
func (idx *Indexer) storeLatestEdit(base hash.Hash, edit hash.Hash, createdAt int64) {
	if idx == nil || base.IsZero() || edit.IsZero() {
		return
	}

	idx.editMu.Lock()
	defer idx.editMu.Unlock()

	entry, exists := idx.latestEdits[base.String()]
	if !exists || createdAt > entry.createdAt || (createdAt == entry.createdAt && edit.String() > entry.hash.String()) {
		idx.latestEdits[base.String()] = editEntry{hash: edit, createdAt: createdAt}
	}
}

// LatestEditFor returns the most recent edit for the given base hash, if any.
func (idx *Indexer) LatestEditFor(base hash.Hash) (hash.Hash, bool) {
	if idx == nil {
		return hash.Hash{}, false
	}

	idx.editMu.RLock()
	defer idx.editMu.RUnlock()

	entry, ok := idx.latestEdits[base.String()]
	if !ok {
		return hash.Hash{}, false
	}
	return entry.hash, true
}

// LookupByComputedID returns all hashes that have the given computed_id.
// Multiple messages may share the same computed_id.
func (idx *Indexer) LookupByComputedID(computedID string) ([]hash.Hash, error) {
	if idx == nil {
		return nil, fmt.Errorf("indexer is nil")
	}

	idx.computedIDMu.RLock()
	defer idx.computedIDMu.RUnlock()

	hashes := idx.computedIDIndex[computedID]
	if len(hashes) == 0 {
		return nil, nil
	}

	// Return a copy to avoid external modification
	result := make([]hash.Hash, len(hashes))
	copy(result, hashes)
	return result, nil
}

// GetComputedID returns the computed_id for a given hash.
func (idx *Indexer) GetComputedID(h hash.Hash) (string, error) {
	if idx == nil {
		return "", fmt.Errorf("indexer is nil")
	}

	idx.computedIDMu.RLock()
	defer idx.computedIDMu.RUnlock()

	return idx.hashToComputedID[h.String()], nil
}

// ListComputedIDs returns all computed_ids that match the given prefix.
// Useful for autocomplete or browsing the ID namespace.
func (idx *Indexer) ListComputedIDs(prefix string) ([]string, error) {
	if idx == nil {
		return nil, fmt.Errorf("indexer is nil")
	}

	idx.computedIDMu.RLock()
	defer idx.computedIDMu.RUnlock()

	var result []string
	for id := range idx.computedIDIndex {
		if strings.HasPrefix(id, prefix) {
			result = append(result, id)
		}
	}
	return result, nil
}
