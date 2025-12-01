package ouroboros

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

// setupTestDir creates a temporary directory for testing
func setupTestDir(tb testing.TB) string { // A
	tb.Helper()
	testDir, err := os.MkdirTemp("", "ouroboros_test_*")
	if err != nil {
		tb.Fatalf("Failed to create test directory: %v", err)
	}
	return testDir
}

// setupTestKeyFile creates a test key file for ouroboros-crypt
func setupTestKeyFile(tb testing.TB, testDir string) string { // A
	tb.Helper()
	// Create a new crypto instance
	asyncCrypt, err := keys.NewAsyncCrypt()
	if err != nil {
		tb.Fatalf("Failed to create async crypt: %v", err)
	}

	// Save the key file
	keyPath := filepath.Join(testDir, "ouroboros.key")
	err = asyncCrypt.SaveToFile(keyPath)
	if err != nil {
		tb.Fatalf("Failed to save key file: %v", err)
	}

	return keyPath
}

// cleanupTestDir removes the test directory
func cleanupTestDir(tb testing.TB, testDir string) { // A
	tb.Helper()
	if err := os.RemoveAll(testDir); err != nil {
		tb.Errorf("Failed to cleanup test directory: %v", err)
	}
}

func testLogger() *slog.Logger { // A
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newStartedDB(t *testing.T, config Config) *OuroborosDB { // A
	t.Helper()

	db, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start OuroborosDB: %v", err)
	}

	t.Cleanup(func() {
		if err := db.CloseWithoutContext(); err != nil {
			t.Errorf("Failed to close OuroborosDB: %v", err)
		}
	})

	return db
}

func TestNewOuroborosDB_Success(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	// Test successful initialization
	db, err := New(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if db == nil {
		t.Fatal("Expected db to be non-nil")
	}

	// Verify struct fields are initialized
	if db.log == nil {
		t.Error("Expected logger to be initialized")
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("Expected Start to succeed, got: %v", err)
	}
	defer func() {
		if err := db.CloseWithoutContext(); err != nil {
			t.Errorf("CloseWithoutContext failed: %v", err)
		}
	}()

	if db.crypt == nil {
		t.Error("Expected crypt to be initialized after Start")
	}
	if _, err := db.kvHandle(); err != nil {
		t.Errorf("Expected kv to be initialized after Start, got: %v", err)
	}
}

func TestStoreData_AutoIndexes(t *testing.T) {
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)
	cfg := Config{Paths: []string{testDir}, MinimumFreeGB: 1, Logger: testLogger()}
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	// ensure indexer is present
	idx := db.Indexer()
	if idx == nil {
		t.Fatalf("expected indexer to be created on Start")
	}

	// store a text payload
	content := []byte("auto index me")
	key, err := db.StoreData(context.Background(), content, StoreOptions{MimeType: "text/plain; charset=utf-8"})
	if err != nil {
		t.Fatalf("store data failed: %v", err)
	}

	// Poll until indexer finds the key (async indexing)
	timeout := time.After(2 * time.Second)
	tick := time.Tick(20 * time.Millisecond)
	found := false
	for {
		select {
		case <-timeout:
			t.Fatalf("timeout waiting for auto-indexing")
		case <-tick:
			res, err := idx.TextSearch("auto index me", 10)
			if err != nil {
				// ignore transient errors; keep polling
				continue
			}
			for _, r := range res {
				if r == key {
					found = true
					break
				}
			}
			if found {
				return
			}
		}
	}
}

func TestGetDataSuggestsLatestEdit(t *testing.T) {
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)
	cfg := Config{Paths: []string{testDir}, MinimumFreeGB: 1, Logger: testLogger()}
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	baseContent := []byte("base")
	baseKey, err := db.StoreData(context.Background(), baseContent, StoreOptions{MimeType: "text/plain; charset=utf-8"})
	if err != nil {
		t.Fatalf("store base: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	editContent := []byte("edit")
	editKey, err := db.StoreData(context.Background(), editContent, StoreOptions{
		MimeType: "text/plain; charset=utf-8",
		EditOf:   baseKey,
	})
	if err != nil {
		t.Fatalf("store edit: %v", err)
	}

	// Force index updates so edit resolution is available synchronously.
	if idx := db.Indexer(); idx != nil {
		if err := idx.IndexHash(baseKey); err != nil {
			t.Fatalf("index base: %v", err)
		}
		if err := idx.IndexHash(editKey); err != nil {
			t.Fatalf("index edit: %v", err)
		}
	}

	data, err := db.GetData(context.Background(), baseKey)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if data.SuggestedEdit.IsZero() || data.SuggestedEdit != editKey {
		t.Fatalf("expected suggested edit %s, got %s", editKey.String(), data.SuggestedEdit.String())
	}
	if data.EditOf != (cryptHash.Hash{}) {
		t.Fatalf("expected base record to have empty EditOf, got %s", data.EditOf.String())
	}
	if string(data.Content) != string(baseContent) {
		t.Fatalf("expected original content to be returned, got %q", string(data.Content))
	}

	resolved, changed := db.resolveLatestEdit(baseKey)
	if !changed || resolved != editKey {
		t.Fatalf("expected resolveLatestEdit to return edit %s, got %s (changed=%v)", editKey.String(), resolved.String(), changed)
	}
}

func TestGetDataSuggestsLatestEditChained(t *testing.T) {
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)
	cfg := Config{Paths: []string{testDir}, MinimumFreeGB: 1, Logger: testLogger()}
	db, err := New(cfg)
	if err != nil {
		t.Fatalf("new db: %v", err)
	}
	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("start db: %v", err)
	}
	defer func() { _ = db.CloseWithoutContext() }()

	// Create original message (Test1)
	test1Content := []byte("Test1")
	test1Key, err := db.StoreData(context.Background(), test1Content, StoreOptions{MimeType: "text/plain; charset=utf-8"})
	if err != nil {
		t.Fatalf("store test1: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Edit Test1 -> Test2
	test2Content := []byte("Test2")
	test2Key, err := db.StoreData(context.Background(), test2Content, StoreOptions{
		MimeType: "text/plain; charset=utf-8",
		EditOf:   test1Key,
	})
	if err != nil {
		t.Fatalf("store test2: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Edit Test2 -> Test3
	test3Content := []byte("Test3")
	test3Key, err := db.StoreData(context.Background(), test3Content, StoreOptions{
		MimeType: "text/plain; charset=utf-8",
		EditOf:   test2Key,
	})
	if err != nil {
		t.Fatalf("store test3: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// Edit Test3 -> Test10
	test10Content := []byte("Test10")
	test10Key, err := db.StoreData(context.Background(), test10Content, StoreOptions{
		MimeType: "text/plain; charset=utf-8",
		EditOf:   test3Key,
	})
	if err != nil {
		t.Fatalf("store test10: %v", err)
	}

	// Force index updates so edit resolution is available synchronously.
	idx := db.Indexer()
	if idx != nil {
		for _, k := range []cryptHash.Hash{test1Key, test2Key, test3Key, test10Key} {
			if err := idx.IndexHash(k); err != nil {
				t.Fatalf("index %s: %v", k.String(), err)
			}
		}
	}

	// Verify the edit chain resolves correctly from Test1 to Test10
	resolved, changed := db.resolveLatestEdit(test1Key)
	if !changed || resolved != test10Key {
		t.Fatalf("expected resolveLatestEdit(test1) to return test10 %s, got %s (changed=%v)",
			test10Key.String(), resolved.String(), changed)
	}

	// Verify GetData on test1 returns suggestedEdit pointing to test10
	data, err := db.GetData(context.Background(), test1Key)
	if err != nil {
		t.Fatalf("get data: %v", err)
	}
	if data.SuggestedEdit.IsZero() || data.SuggestedEdit != test10Key {
		t.Fatalf("expected suggested edit %s, got %s", test10Key.String(), data.SuggestedEdit.String())
	}

	// Verify the original content is returned (not the edit content)
	if string(data.Content) != string(test1Content) {
		t.Fatalf("expected original content %q, got %q", string(test1Content), string(data.Content))
	}

	// Verify that intermediate edits also resolve to test10
	resolvedFrom2, _ := db.resolveLatestEdit(test2Key)
	if resolvedFrom2 != test10Key {
		t.Fatalf("expected resolveLatestEdit(test2) to return test10 %s, got %s",
			test10Key.String(), resolvedFrom2.String())
	}

	resolvedFrom3, _ := db.resolveLatestEdit(test3Key)
	if resolvedFrom3 != test10Key {
		t.Fatalf("expected resolveLatestEdit(test3) to return test10 %s, got %s",
			test10Key.String(), resolvedFrom3.String())
	}
}

func TestNewOuroborosDB_WithDefaultLogger(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        nil, // Test default logger initialization
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if db.log == nil {
		t.Error("Expected default logger to be initialized")
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("Expected Start to succeed, got: %v", err)
	}

	if err := db.CloseWithoutContext(); err != nil {
		t.Fatalf("CloseWithoutContext failed: %v", err)
	}
}

func TestNewOuroborosDB_EmptyPaths(t *testing.T) { // A
	config := Config{
		Paths:         []string{}, // Empty paths should cause error
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db, err := New(config)
	if err == nil {
		t.Fatal("Expected error for empty paths, got nil")
	}
	if db != nil {
		t.Error("Expected db to be nil when error occurs")
	}

	expectedErr := "at least one path must be provided in config"
	if err.Error() != expectedErr {
		t.Errorf("Expected error message '%s', got '%s'", expectedErr, err.Error())
	}
}

func TestNewOuroborosDB_MissingKeyFile(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Don't create key file - should cause error
	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Expected New to succeed, got: %v", err)
	}

	if db == nil {
		t.Fatal("Expected db to be non-nil")
	}

	err = db.Start(context.Background())
	if err == nil {
		t.Fatal("Expected error for missing key file, got nil")
	}

	if !strings.Contains(err.Error(), "init crypt") {
		t.Errorf("Expected error to mention crypt init, got: %v", err)
	}
}

func TestNewOuroborosDB_InvalidPath(t *testing.T) { // A
	// Use a path that doesn't exist and cannot be created
	invalidPath := "/invalid/nonexistent/path"

	config := Config{
		Paths:         []string{invalidPath},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Expected New to succeed, got: %v", err)
	}

	err = db.Start(context.Background())
	if err == nil {
		t.Fatal("Expected error for invalid path, got nil")
	}

	if !strings.Contains(err.Error(), "mkdir") {
		t.Errorf("Expected error to mention mkdir, got: %v", err)
	}
}

func TestOuroborosDB_BasicKVOperations(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	// Test basic KV operations
	testContent := []byte("test content")
	testData := ouroboroskv.Data{
		Content:        testContent,
		RSDataSlices:   4,
		RSParitySlices: 2,
	}

	kv, err := db.kvHandle()
	if err != nil {
		t.Fatalf("failed to get kv handle: %v", err)
	}

	// Write data
	dataKey, err := kv.WriteData(testData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Check if data exists
	exists, err := kv.DataExists(dataKey)
	if err != nil {
		t.Fatalf("Failed to check data existence: %v", err)
	}
	if !exists {
		t.Error("Expected data to exist after writing")
	}

	// Read data back
	readData, err := kv.ReadData(dataKey)
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}

	// Verify content
	if string(readData.Content) != string(testData.Content) {
		t.Errorf("Expected content '%s', got '%s'", string(testData.Content), string(readData.Content))
	}
}

func TestStoreAndGetData_Text(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	content := []byte("hello mime aware world")

	key, err := db.StoreData(context.Background(), content, StoreOptions{MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("StoreData failed: %v", err)
	}

	retrieved, err := db.GetData(context.Background(), key)
	if err != nil {
		t.Fatalf("GetData failed: %v", err)
	}

	if !retrieved.IsText {
		t.Fatalf("expected retrieved data to be marked as text")
	}
	if retrieved.MimeType != "text/plain" {
		t.Fatalf("expected mime type \"text/plain\" for text data, got %q", retrieved.MimeType)
	}
	if !bytes.Equal(retrieved.Content, content) {
		t.Fatalf("expected content to match original")
	}
}

func TestStoreAndGetData_Binary(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	content := []byte{0xde, 0xad, 0xbe, 0xef}
	mimeType := "application/octet-stream"

	key, err := db.StoreData(context.Background(), content, StoreOptions{MimeType: mimeType})
	if err != nil {
		t.Fatalf("StoreData failed: %v", err)
	}

	retrieved, err := db.GetData(context.Background(), key)
	if err != nil {
		t.Fatalf("GetData failed: %v", err)
	}

	if retrieved.IsText {
		t.Fatalf("expected binary data to be marked as non-text")
	}
	if retrieved.MimeType != mimeType {
		t.Fatalf("expected mime type %q, got %q", mimeType, retrieved.MimeType)
	}
	if !bytes.Equal(retrieved.Content, content) {
		t.Fatalf("expected retrieved content to match original")
	}
}

func TestStoreDataParentChildRelationships(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	parentContent := []byte("parent payload")
	parentKey, err := db.StoreData(context.Background(), parentContent, StoreOptions{})
	if err != nil {
		t.Fatalf("StoreData for parent failed: %v", err)
	}

	childContent := []byte("child payload")
	childKey, err := db.StoreData(context.Background(), childContent, StoreOptions{Parent: parentKey})
	if err != nil {
		t.Fatalf("StoreData for child failed: %v", err)
	}

	parentRetrieved, err := db.GetData(context.Background(), parentKey)
	if err != nil {
		t.Fatalf("GetData for parent failed: %v", err)
	}

	if parentRetrieved.CreatedAt.IsZero() {
		t.Fatal("expected parent data to include creation timestamp")
	}

	if len(parentRetrieved.Children) != 1 {
		t.Fatalf("expected parent to have 1 child, got %d", len(parentRetrieved.Children))
	}
	if parentRetrieved.Children[0] != childKey {
		t.Fatalf("expected parent child to be %s, got %s", childKey.String(), parentRetrieved.Children[0].String())
	}

	childRetrieved, err := db.GetData(context.Background(), childKey)
	if err != nil {
		t.Fatalf("GetData for child failed: %v", err)
	}

	if childRetrieved.CreatedAt.IsZero() {
		t.Fatal("expected child data to include creation timestamp")
	}

	if childRetrieved.Parent != parentKey {
		t.Fatalf("expected child parent to be %s, got %s", parentKey.String(), childRetrieved.Parent.String())
	}

	roots, err := db.ListData(context.Background())
	if err != nil {
		t.Fatalf("ListData failed: %v", err)
	}
	if len(roots) != 1 || roots[0] != parentKey {
		t.Fatalf("expected ListData to return parent root key only, got %v", roots)
	}

	children, err := db.ListChildren(context.Background(), parentKey)
	if err != nil {
		t.Fatalf("ListChildren failed: %v", err)
	}
	if len(children) != 1 || children[0] != childKey {
		t.Fatalf("expected ListChildren to return child key, got %v", children)
	}
}

func TestOuroborosDB_CryptOperations(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	// Test encryption/decryption
	testData := []byte("secret message for encryption test")

	// Encrypt data
	encrypted, err := db.crypt.Encrypt(testData)
	if err != nil {
		t.Fatalf("Failed to encrypt data: %v", err)
	}

	// Decrypt data
	decrypted, err := db.crypt.Decrypt(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt data: %v", err)
	}

	// Verify decrypted data matches original
	if string(decrypted) != string(testData) {
		t.Errorf("Expected decrypted data '%s', got '%s'", string(testData), string(decrypted))
	}
}

func TestOuroborosDB_HashOperations(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db := newStartedDB(t, config)

	// Test hashing
	testData := []byte("data to hash")
	hash1 := db.crypt.HashBytes(testData)
	hash2 := db.crypt.HashBytes(testData)

	// Same data should produce same hash
	if hash1 != hash2 {
		t.Error("Same data should produce same hash")
	}

	// Different data should produce different hash
	differentData := []byte("different data to hash")
	hash3 := db.crypt.HashBytes(differentData)
	if hash1 == hash3 {
		t.Error("Different data should produce different hash")
	}
}

func TestOuroborosDB_Close(t *testing.T) { // A
	testDir := setupTestDir(t)
	t.Cleanup(func() { cleanupTestDir(t, testDir) })

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	db, err := New(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}

	if err := db.Start(context.Background()); err != nil {
		t.Fatalf("Failed to start OuroborosDB: %v", err)
	}

	// Close should not panic and should be callable multiple times
	if err := db.CloseWithoutContext(); err != nil {
		t.Fatalf("Expected first close to succeed, got: %v", err)
	}
	if err := db.CloseWithoutContext(); err != nil {
		t.Fatalf("Expected second close to succeed, got: %v", err)
	}
}

// testDB returns a minimal OuroborosDB instance for testing encode/decode functions
func testDB(t *testing.T) *OuroborosDB { // A
	t.Helper()
	return &OuroborosDB{
		log: testLogger(),
	}
}

// Benchmark test for database creation
func BenchmarkNewOuroborosDB(b *testing.B) { // A
	testDir := setupTestDir(b)
	b.Cleanup(func() { cleanupTestDir(b, testDir) })

	setupTestKeyFile(b, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        testLogger(),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := New(config)
		if err != nil {
			b.Fatalf("Failed to create OuroborosDB: %v", err)
		}
		if err := db.Start(ctx); err != nil {
			b.Fatalf("Failed to start OuroborosDB: %v", err)
		}
		if err := db.CloseWithoutContext(); err != nil {
			b.Fatalf("Failed to close OuroborosDB: %v", err)
		}
	}
}
