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
func setupTestDir(tb testing.TB) string {
	tb.Helper()
	testDir, err := os.MkdirTemp("", "ouroboros_test_*")
	if err != nil {
		tb.Fatalf("Failed to create test directory: %v", err)
	}
	return testDir
}

// setupTestKeyFile creates a test key file for ouroboros-crypt
func setupTestKeyFile(tb testing.TB, testDir string) string {
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
func cleanupTestDir(tb testing.TB, testDir string) {
	tb.Helper()
	if err := os.RemoveAll(testDir); err != nil {
		tb.Errorf("Failed to cleanup test directory: %v", err)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newStartedDB(t *testing.T, config Config) *OuroborosDB {
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

func TestNewOuroborosDB_Success(t *testing.T) {
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

func TestNewOuroborosDB_WithDefaultLogger(t *testing.T) {
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

func TestNewOuroborosDB_EmptyPaths(t *testing.T) {
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

func TestNewOuroborosDB_MissingKeyFile(t *testing.T) {
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

func TestNewOuroborosDB_InvalidPath(t *testing.T) {
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

func TestOuroborosDB_BasicKVOperations(t *testing.T) {
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

func TestStoreAndGetData_Text(t *testing.T) {
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

func TestStoreAndGetData_Binary(t *testing.T) {
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

func TestStoreDataParentChildRelationships(t *testing.T) {
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

func TestOuroborosDB_CryptOperations(t *testing.T) {
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

func TestOuroborosDB_HashOperations(t *testing.T) {
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

func TestOuroborosDB_Close(t *testing.T) {
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
func testDB(t *testing.T) *OuroborosDB {
	t.Helper()
	return &OuroborosDB{
		log: testLogger(),
	}
}

func TestEncodeContentWithMimeType(t *testing.T) {
	db := testDB(t)
	content := []byte("sample payload")

	toLongMime := strings.Repeat("a", payloadHeaderContentSizeLen+1)

	tests := []struct {
		name     string
		mime     string
		expected bool // whether a error should be expected
		length   int
	}{
		{"mime", "text/plain", false, payloadHeaderSize + len(content)},
		{"empty", "", false, len(content) + 1},
		{"whitespace", "   ", false, len(content) + 1},
		{"tabs and newlines", "\n\t  \r", false, len(content) + 1},
		{"mixed whitespace", "  \t \n ", false, len(content) + 1},
		{"mime too long", toLongMime, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := encodeContentWithMimeType(content, tc.mime)

			if tc.expected {
				// expecting an error
				if err == nil {
					t.Fatalf("expected error for mime %q but got none", tc.mime)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for mime %q: %v", tc.mime, err)
			}

			if got := len(encoded); got != tc.length {
				t.Fatalf("unexpected encoded length for mime %q: want %d, got %d", tc.mime, tc.length, got)
			}

			// Validate header byte and payload depending on whether we expect a MIME header
			trimmed := strings.TrimSpace(tc.mime)
			if trimmed == "" {
				// no mime: should be single-byte header followed by content
				if encoded[0] != payloadHeaderNotExisting {
					t.Fatalf("expected header byte 0x%02x, got 0x%02x for mime %q", payloadHeaderNotExisting, encoded[0], tc.mime)
				}
				if !bytes.Equal(encoded[1:], content) {
					t.Fatalf("expected content to follow single-byte header for mime %q", tc.mime)
				}
				// Use decodeContent to confirm round-trip behavior
				decoded, payloadHeader, isMime, decErr := db.decodeContent(encoded)
				if decErr != nil {
					t.Fatalf("decodeContent failed: %v", decErr)
				}
				if !bytes.Equal(decoded, content) {
					t.Fatalf("decoded content mismatch for mime %q: want %q got %q", tc.mime, content, decoded)
				}
				if isMime {
					t.Fatalf("expected isMime false for no-mime case, got true")
				}
				if payloadHeader != nil {
					t.Fatalf("expected nil payloadHeader for no-mime case, got %v", payloadHeader)
				}
			} else {
				// With a MIME header we expect a full fixed-size header and trimmed mime in it
				if encoded[0] != payloadHeaderIsMime {
					t.Fatalf("expected header byte 0x%02x, got 0x%02x for mime %q", payloadHeaderIsMime, encoded[0], tc.mime)
				}
				headMime := string(bytes.TrimRight(encoded[1:payloadHeaderSize], "\x00"))
				if strings.TrimSpace(headMime) != trimmed {
					t.Fatalf("header mime mismatch for mime %q: expected %q got %q", tc.mime, trimmed, headMime)
				}
				if !bytes.Equal(encoded[payloadHeaderSize:], content) {
					t.Fatalf("expected content after payload header for mime %q", tc.mime)
				}
				// decodeContent should return the trimmed mime in payloadHeader
				decoded, payloadHeader, isMime, decErr := db.decodeContent(encoded)
				if decErr != nil {
					t.Fatalf("decodeContent failed: %v", decErr)
				}
				if !bytes.Equal(decoded, content) {
					t.Fatalf("decoded content mismatch for mime %q: want %q got %q", tc.mime, content, decoded)
				}
				if !isMime {
					t.Fatalf("expected isMime true for mime case, got false")
				}
				mimeOut := strings.TrimSpace(string(bytes.TrimRight(payloadHeader, "\x00")))
				if mimeOut != trimmed {
					t.Fatalf("decoded mime mismatch for mime %q: want %q got %q", tc.mime, trimmed, mimeOut)
				}
			}
		})
	}
}

func TestDecodeContent(t *testing.T) {
	db := testDB(t)
	content := []byte("test payload data")

	tests := []struct {
		name         string
		payload      []byte
		expectError  bool
		expectData   []byte
		expectMime   string
		expectIsMime bool
	}{
		{
			name:        "empty_payload",
			payload:     []byte{},
			expectError: true,
		},
		{
			name:         "single_byte_no_header",
			payload:      []byte{payloadHeaderNotExisting},
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "",
			expectIsMime: false,
		},
		{
			name:         "no_header_with_content",
			payload:      append([]byte{payloadHeaderNotExisting}, content...),
			expectError:  false,
			expectData:   content,
			expectMime:   "",
			expectIsMime: false,
		},
		{
			name:        "invalid_flag_byte",
			payload:     []byte{0xFF, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "existing_flag_but_no_mime_flag",
			payload:     append([]byte{payloadHeaderExisting}, content...),
			expectError: true,
		},
		{
			name:        "mime_flag_but_payload_too_short",
			payload:     []byte{payloadHeaderIsMime, 't', 'e', 'x', 't'},
			expectError: true,
		},
		{
			name: "mime_header_exactly_at_boundary",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "",
			expectIsMime: true,
		},
		{
			name: "valid_mime_header_with_text/plain",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("text/plain"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "text/plain",
			expectIsMime: true,
		},
		{
			name: "valid_mime_header_with_application/json",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/json"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "application/json",
			expectIsMime: true,
		},
		{
			name: "mime_header_with_padding_zeros",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("image/png"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "image/png",
			expectIsMime: true,
		},
		{
			name: "mime_header_filled_to_maximum",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				maxMime := strings.Repeat("a", payloadHeaderContentSizeLen)
				copy(header[1:], []byte(maxMime))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   strings.Repeat("a", payloadHeaderContentSizeLen),
			expectIsMime: true,
		},
		{
			name: "mime_header_with_empty_content",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("text/html"))
				return header
			}(),
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "text/html",
			expectIsMime: true,
		},
		{
			name: "mime_header_with_binary_content",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/octet-stream"))
				binaryData := []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF}
				return append(header, binaryData...)
			}(),
			expectError:  false,
			expectData:   []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF},
			expectMime:   "application/octet-stream",
			expectIsMime: true,
		},
		{
			name:        "random_invalid_flag_combinations",
			payload:     []byte{0x30, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "flag_0x01_invalid",
			payload:     []byte{0x01, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "flag_0x11_invalid",
			payload:     []byte{0x11, 0x01, 0x02},
			expectError: true,
		},
		{
			name: "payloadHeaderExisting_flag_with_full_header_but_should_fail",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderExisting
				copy(header[1:], []byte("text/plain"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "text/plain",
			expectIsMime: false,
		},
		{
			name: "mime_with_special_characters",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/vnd.api+json"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "application/vnd.api+json",
			expectIsMime: true,
		},
		{
			name: "mime_header_exactly_payloadHeaderSize_bytes",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				return header
			}(),
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "",
			expectIsMime: true,
		},
		{
			name: "mime_header_one_byte_short",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize-1)
				header[0] = payloadHeaderIsMime
				return header
			}(),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, payloadHeader, isMime, err := db.decodeContent(tc.payload)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !bytes.Equal(data, tc.expectData) {
				t.Fatalf("data mismatch: want %q, got %q", tc.expectData, data)
			}

			if isMime != tc.expectIsMime {
				t.Fatalf("isMime mismatch: want %v, got %v", tc.expectIsMime, isMime)
			}

			if tc.expectIsMime || tc.payload[0] == payloadHeaderExisting {
				actualMime := string(bytes.TrimRight(payloadHeader, "\x00"))
				if actualMime != tc.expectMime {
					t.Fatalf("mime type mismatch: want %q, got %q", tc.expectMime, actualMime)
				}
			} else {
				if payloadHeader != nil {
					t.Fatalf("expected nil payloadHeader for non-mime case, got %v", payloadHeader)
				}
			}
		})
	}
}

// Benchmark test for database creation
func BenchmarkNewOuroborosDB(b *testing.B) {
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
