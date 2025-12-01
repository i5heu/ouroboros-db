package index

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"log/slog"

	crypt "github.com/i5heu/ouroboros-crypt"
	cryptkeys "github.com/i5heu/ouroboros-crypt/pkg/keys"

	// MIME type is now stored in metadata
	meta "github.com/i5heu/ouroboros-db/pkg/meta"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

func testLogger() *slog.Logger {
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{})
	return slog.New(h)
}

func setupTestDir(tb testing.TB) string {
	tb.Helper()
	testDir, err := os.MkdirTemp("", "ouroboros_index_test_*")
	if err != nil {
		tb.Fatalf("failed to create tmp dir: %v", err)
	}
	return testDir
}

func setupTestKeyFile(tb testing.TB, testDir string) string {
	tb.Helper()
	asyncCrypt, err := cryptkeys.NewAsyncCrypt()
	if err != nil {
		tb.Fatalf("Failed to create async crypt: %v", err)
	}
	keyPath := filepath.Join(testDir, "ouroboros.key")
	if err := asyncCrypt.SaveToFile(keyPath); err != nil {
		tb.Fatalf("Failed to save key file: %v", err)
	}
	return keyPath
}

func newInitializedKV(tb testing.TB, dir string) (ouroboroskv.Store, func()) {
	tb.Helper()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			tb.Fatalf("mkdir: %v", err)
		}
	}
	keyPath := setupTestKeyFile(tb, dir)

	c, err := crypt.NewFromFile(keyPath)
	if err != nil {
		tb.Fatalf("crypt new from file: %v", err)
	}

	cfg := &ouroboroskv.Config{
		Paths:            []string{filepath.Join(dir, "kv")},
		MinimumFreeSpace: 1,
		Logger:           testLogger(),
	}
	kv, err := ouroboroskv.Init(c, cfg)
	if err != nil {
		tb.Fatalf("init kv: %v", err)
	}
	cleanup := func() { _ = kv.Close() }
	return kv, cleanup
}

func TestIndexer_IndexSearchRemove_LastChildActivity(t *testing.T) {
	tmp := setupTestDir(t)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	kv, cleanup := newInitializedKV(t, tmp)
	t.Cleanup(cleanup)

	idx := NewIndexer(kv, testLogger())
	defer idx.Close()

	// store a text payload by writing raw data to KV with mime in meta
	content := []byte("hello world from test indexer")
	encContent := content
	metaBytes, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	data := ouroboroskv.Data{Content: encContent, Meta: metaBytes, RSDataSlices: 4, RSParitySlices: 2}
	key, err := kv.WriteData(data)
	if err != nil {
		t.Fatalf("store data: %v", err)
	}

	if err := idx.IndexHash(key); err != nil {
		t.Fatalf("index hash: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	res, err := idx.TextSearch("hello", 10)
	if err != nil {
		t.Fatalf("text search: %v", err)
	}
	if len(res) != 1 || res[0] != key {
		t.Fatalf("expected to find key in search, got %v", res)
	}

	// Remove and ensure search returns nothing
	if err := idx.RemoveHash(key); err != nil {
		t.Fatalf("remove hash: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	res2, err := idx.TextSearch("hello", 10)
	if err != nil {
		t.Fatalf("text search after remove: %v", err)
	}
	if len(res2) != 0 {
		t.Fatalf("expected no results after removal, got %v", res2)
	}

	// Test LastChildActivity
	encParentContent := []byte("parent")
	metaParent, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("marshal meta parent: %v", err)
	}
	p := ouroboroskv.Data{Content: encParentContent, Meta: metaParent, RSDataSlices: 4, RSParitySlices: 2}
	parentKey, err := kv.WriteData(p)
	if err != nil {
		t.Fatalf("store parent: %v", err)
	}

	// create two children with a short delay so we can detect last activity
	encChild1 := []byte("child1")
	metaChild1, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("marshal child1 meta: %v", err)
	}
	d1 := ouroboroskv.Data{Content: encChild1, Meta: metaChild1, Parent: parentKey, RSDataSlices: 4, RSParitySlices: 2}
	if _, err = kv.WriteData(d1); err != nil {
		t.Fatalf("store child1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	encChild2 := []byte("child2")
	metaChild2, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("marshal child2 meta: %v", err)
	}
	d2 := ouroboroskv.Data{Content: encChild2, Meta: metaChild2, Parent: parentKey, RSDataSlices: 4, RSParitySlices: 2}
	child2, err := kv.WriteData(d2)
	if err != nil {
		t.Fatalf("store child2: %v", err)
	}

	if err := idx.IndexHash(parentKey); err != nil {
		t.Fatalf("index parent: %v", err)
	}

	lastActivity, err := idx.LastChildActivity(parentKey)
	if err != nil {
		t.Fatalf("last child activity: %v", err)
	}
	ch2, err := kv.ReadData(child2)
	if err != nil {
		t.Fatalf("read child2: %v", err)
	}
	var md meta.Metadata
	if err := json.Unmarshal(ch2.Meta, &md); err != nil {
		t.Fatalf("unmarshal child2 metadata: %v", err)
	}
	t2 := md.CreatedAt
	if err != nil {
		t.Fatalf("parse child2 createdAt: %v", err)
	}
	if lastActivity != t2.UnixNano() {
		t.Fatalf("expected last child activity %d, got %d", t2.UnixNano(), lastActivity)
	}
}

func TestIndexer_ComputedID(t *testing.T) {
	tmp := setupTestDir(t)
	t.Cleanup(func() { _ = os.RemoveAll(tmp) })

	kv, cleanup := newInitializedKV(t, tmp)
	t.Cleanup(cleanup)

	idx := NewIndexer(kv, testLogger())
	defer idx.Close()

	// Create a root message with title "thoughts"
	rootMeta, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain", Title: "thoughts"})
	if err != nil {
		t.Fatalf("marshal root meta: %v", err)
	}
	rootData := ouroboroskv.Data{Content: []byte("root content"), Meta: rootMeta, RSDataSlices: 4, RSParitySlices: 2}
	rootKey, err := kv.WriteData(rootData)
	if err != nil {
		t.Fatalf("store root: %v", err)
	}
	if err := idx.IndexHash(rootKey); err != nil {
		t.Fatalf("index root: %v", err)
	}

	// Verify root has computed_id "thoughts"
	rootComputedID, err := idx.GetComputedID(rootKey)
	if err != nil {
		t.Fatalf("get root computed_id: %v", err)
	}
	if rootComputedID != "thoughts" {
		t.Fatalf("expected root computed_id 'thoughts', got %q", rootComputedID)
	}

	// Create a child message with title "gravitation" under root
	childMeta, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain", Title: "gravitation"})
	if err != nil {
		t.Fatalf("marshal child meta: %v", err)
	}
	childData := ouroboroskv.Data{Content: []byte("child content"), Meta: childMeta, Parent: rootKey, RSDataSlices: 4, RSParitySlices: 2}
	childKey, err := kv.WriteData(childData)
	if err != nil {
		t.Fatalf("store child: %v", err)
	}
	if err := idx.IndexHash(childKey); err != nil {
		t.Fatalf("index child: %v", err)
	}

	// Verify child has computed_id "thoughts:gravitation"
	childComputedID, err := idx.GetComputedID(childKey)
	if err != nil {
		t.Fatalf("get child computed_id: %v", err)
	}
	if childComputedID != "thoughts:gravitation" {
		t.Fatalf("expected child computed_id 'thoughts:gravitation', got %q", childComputedID)
	}

	// Create a grandchild message with title "physics" under child
	grandchildMeta, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain", Title: "physics"})
	if err != nil {
		t.Fatalf("marshal grandchild meta: %v", err)
	}
	grandchildData := ouroboroskv.Data{Content: []byte("grandchild content"), Meta: grandchildMeta, Parent: childKey, RSDataSlices: 4, RSParitySlices: 2}
	grandchildKey, err := kv.WriteData(grandchildData)
	if err != nil {
		t.Fatalf("store grandchild: %v", err)
	}
	if err := idx.IndexHash(grandchildKey); err != nil {
		t.Fatalf("index grandchild: %v", err)
	}

	// Verify grandchild has computed_id "thoughts:gravitation:physics"
	grandchildComputedID, err := idx.GetComputedID(grandchildKey)
	if err != nil {
		t.Fatalf("get grandchild computed_id: %v", err)
	}
	if grandchildComputedID != "thoughts:gravitation:physics" {
		t.Fatalf("expected grandchild computed_id 'thoughts:gravitation:physics', got %q", grandchildComputedID)
	}

	// Verify LookupByComputedID works
	found, err := idx.LookupByComputedID("thoughts:gravitation:physics")
	if err != nil {
		t.Fatalf("lookup by computed_id: %v", err)
	}
	if len(found) != 1 || found[0].String() != grandchildKey.String() {
		t.Fatalf("expected to find grandchild key, got %v", found)
	}

	// Create another message with the same computed_id (same title chain)
	anotherMeta, err := json.Marshal(meta.Metadata{CreatedAt: time.Now().UTC(), MimeType: "text/plain", Title: "physics"})
	if err != nil {
		t.Fatalf("marshal another meta: %v", err)
	}
	anotherData := ouroboroskv.Data{Content: []byte("another physics content"), Meta: anotherMeta, Parent: childKey, RSDataSlices: 4, RSParitySlices: 2}
	anotherKey, err := kv.WriteData(anotherData)
	if err != nil {
		t.Fatalf("store another: %v", err)
	}
	if err := idx.IndexHash(anotherKey); err != nil {
		t.Fatalf("index another: %v", err)
	}

	// Verify both messages with same computed_id are found
	foundMultiple, err := idx.LookupByComputedID("thoughts:gravitation:physics")
	if err != nil {
		t.Fatalf("lookup by computed_id for multiple: %v", err)
	}
	if len(foundMultiple) != 2 {
		t.Fatalf("expected 2 keys with same computed_id, got %d", len(foundMultiple))
	}

	// Verify ListComputedIDs works with prefix
	ids, err := idx.ListComputedIDs("thoughts:")
	if err != nil {
		t.Fatalf("list computed_ids: %v", err)
	}
	if len(ids) != 2 { // "thoughts:gravitation" and "thoughts:gravitation:physics"
		t.Fatalf("expected 2 computed_ids with prefix 'thoughts:', got %d: %v", len(ids), ids)
	}
}
