package ouroboros

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/i5heu/ouroboros-crypt/keys"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
	"github.com/sirupsen/logrus"
)

// setupTestDir creates a temporary directory for testing
func setupTestDir(t *testing.T) string {
	testDir, err := os.MkdirTemp("", "ouroboros_test_*")
	if err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}
	return testDir
}

// setupTestKeyFile creates a test key file for ouroboros-crypt
func setupTestKeyFile(t *testing.T, testDir string) string {
	// Create a new crypto instance
	asyncCrypt, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("Failed to create async crypt: %v", err)
	}

	// Save the key file
	keyPath := filepath.Join(testDir, "ouroboros.key")
	err = asyncCrypt.SaveToFile(keyPath)
	if err != nil {
		t.Fatalf("Failed to save key file: %v", err)
	}

	return keyPath
}

// cleanupTestDir removes the test directory
func cleanupTestDir(t *testing.T, testDir string) {
	err := os.RemoveAll(testDir)
	if err != nil {
		t.Errorf("Failed to cleanup test directory: %v", err)
	}
}

func TestNewOuroborosDB_Success(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	// Create a test logger
	logger := logrus.New()
	logger.SetOutput(os.Stdout)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logger,
	}

	// Test successful initialization
	db, err := NewOuroborosDB(config)
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
	if db.crypt == nil {
		t.Error("Expected crypt to be initialized")
	}
	if db.kv == nil {
		t.Error("Expected kv to be initialized")
	}

	// Test Close function
	db.Close()
}

func TestNewOuroborosDB_WithDefaultLogger(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        nil, // Test default logger initialization
	}

	db, err := NewOuroborosDB(config)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if db.log == nil {
		t.Error("Expected default logger to be initialized")
	}

	db.Close()
}

func TestNewOuroborosDB_EmptyPaths(t *testing.T) {
	config := Config{
		Paths:         []string{}, // Empty paths should cause error
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
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
	defer cleanupTestDir(t, testDir)

	// Don't create key file - should cause error
	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err == nil {
		t.Fatal("Expected error for missing key file, got nil")
	}
	if db != nil {
		t.Error("Expected db to be nil when error occurs")
	}

	// Check that error mentions ouroboros-crypt
	expectedPrefix := "failed to initialize ouroboros-crypt:"
	if len(err.Error()) < len(expectedPrefix) || err.Error()[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("Expected error to start with '%s', got: %v", expectedPrefix, err)
	}
}

func TestNewOuroborosDB_InvalidPath(t *testing.T) {
	// Use a path that doesn't exist and cannot be created
	invalidPath := "/invalid/nonexistent/path"

	// Create key file in a temporary location
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{invalidPath},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err == nil {
		t.Fatal("Expected error for invalid path, got nil")
	}
	if db != nil {
		t.Error("Expected db to be nil when error occurs")
	}
}

func TestOuroborosDB_BasicKVOperations(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}
	defer db.Close()

	// Test basic KV operations
	testContent := []byte("test content")
	testData := ouroboroskv.Data{
		Key:                     db.crypt.HashBytes(testContent),
		Content:                 testContent,
		ReedSolomonShards:       4,
		ReedSolomonParityShards: 2,
	}

	// Write data
	err = db.kv.WriteData(testData)
	if err != nil {
		t.Fatalf("Failed to write data: %v", err)
	}

	// Use the key from the testData
	key := testData.Key

	// Check if data exists
	exists, err := db.kv.DataExists(key)
	if err != nil {
		t.Fatalf("Failed to check data existence: %v", err)
	}
	if !exists {
		t.Error("Expected data to exist after writing")
	}

	// Read data back
	readData, err := db.kv.ReadData(key)
	if err != nil {
		t.Fatalf("Failed to read data: %v", err)
	}

	// Verify content
	if string(readData.Content) != string(testData.Content) {
		t.Errorf("Expected content '%s', got '%s'", string(testData.Content), string(readData.Content))
	}
}

func TestOuroborosDB_CryptOperations(t *testing.T) {
	testDir := setupTestDir(t)
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}
	defer db.Close()

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
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}
	defer db.Close()

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
	defer cleanupTestDir(t, testDir)

	// Setup test key file
	setupTestKeyFile(t, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	db, err := NewOuroborosDB(config)
	if err != nil {
		t.Fatalf("Failed to create OuroborosDB: %v", err)
	}

	// Close should not panic and should be callable multiple times
	db.Close()
	db.Close() // Should not panic on second call
}

// Benchmark test for database creation
func BenchmarkNewOuroborosDB(b *testing.B) {
	testDir := setupTestDir(&testing.T{})
	defer cleanupTestDir(&testing.T{}, testDir)

	setupTestKeyFile(&testing.T{}, testDir)

	config := Config{
		Paths:         []string{testDir},
		MinimumFreeGB: 1,
		Logger:        logrus.New(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db, err := NewOuroborosDB(config)
		if err != nil {
			b.Fatalf("Failed to create OuroborosDB: %v", err)
		}
		db.Close()
	}
}
