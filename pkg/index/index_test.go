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
	enc "github.com/i5heu/ouroboros-db/pkg/encoding"
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

func newInitializedKV(tb testing.TB, dir string) (*ouroboroskv.KV, func()) {
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

	idx := NewIndexer(kv)
	defer idx.Close()

	// store a text payload by writing raw data to KV (with encoding)
	content := []byte("hello world from test indexer")
	encContent, err := enc.EncodeContentWithMimeType(content, "text/plain")
	if err != nil {
		t.Fatalf("encode content: %v", err)
	}
	metaBytes, err := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
	}{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatalf("marshal meta: %v", err)
	}
	data := ouroboroskv.Data{Content: encContent, MetaData: metaBytes, RSDataSlices: 4, RSParitySlices: 2}
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
	encParentContent, err := enc.EncodeContentWithMimeType([]byte("parent"), "text/plain")
	if err != nil {
		t.Fatalf("encode parent content: %v", err)
	}
	metaParent, err := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
	}{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatalf("marshal meta parent: %v", err)
	}
	p := ouroboroskv.Data{Content: encParentContent, MetaData: metaParent, RSDataSlices: 4, RSParitySlices: 2}
	parentKey, err := kv.WriteData(p)
	if err != nil {
		t.Fatalf("store parent: %v", err)
	}

	// create two children with a short delay so we can detect last activity
	encChild1, err := enc.EncodeContentWithMimeType([]byte("child1"), "text/plain")
	if err != nil {
		t.Fatalf("encode child1: %v", err)
	}
	metaChild1, err := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
	}{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatalf("marshal child1 meta: %v", err)
	}
	d1 := ouroboroskv.Data{Content: encChild1, MetaData: metaChild1, Parent: parentKey, RSDataSlices: 4, RSParitySlices: 2}
	if _, err = kv.WriteData(d1); err != nil {
		t.Fatalf("store child1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	encChild2, err := enc.EncodeContentWithMimeType([]byte("child2"), "text/plain")
	if err != nil {
		t.Fatalf("encode child2: %v", err)
	}
	metaChild2, err := json.Marshal(struct {
		CreatedAt string `json:"created_at"`
	}{CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		t.Fatalf("marshal child2 meta: %v", err)
	}
	d2 := ouroboroskv.Data{Content: encChild2, MetaData: metaChild2, Parent: parentKey, RSDataSlices: 4, RSParitySlices: 2}
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
	var meta struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(ch2.MetaData, &meta); err != nil {
		t.Fatalf("unmarshal child2 metadata: %v", err)
	}
	t2, err := time.Parse(time.RFC3339Nano, meta.CreatedAt)
	if err != nil {
		t.Fatalf("parse child2 createdAt: %v", err)
	}
	if lastActivity != t2.UnixNano() {
		t.Fatalf("expected last child activity %d, got %d", t2.UnixNano(), lastActivity)
	}
}
