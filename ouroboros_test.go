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

	"github.com/i5heu/ouroboros-crypt/keys"
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
