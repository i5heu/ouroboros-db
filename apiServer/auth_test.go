package apiServer

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/pkg/hash"
	ouroboros "github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/browserCrypt"
)

// Test defaultAuth function

func TestDefaultAuth_Success(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	// Create a test auth key and store it in the database
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	nonce := []byte("123456789012") // 12 bytes for GCM
	plaintext := []byte("auth:test-token")

	// Encrypt the plaintext with the key
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("failed to create GCM: %v", err)
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	// Create BrowserKey and store in DB
	browserKey := browserCrypt.BrowserKey{
		Key:       key,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Nonce:     nil,
	}

	keyData, err := json.Marshal(browserKey)
	if err != nil {
		t.Fatalf("failed to marshal browser key: %v", err)
	}

	keyHash, err := db.StoreData(context.Background(), keyData, ouroboros.StoreOptions{})
	if err != nil {
		t.Fatalf("failed to store key: %v", err)
	}

	// Create request with proper headers
	// The keyHash is already a cryptHash.Hash, just encode its bytes to base64
	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", string(ciphertext))
	req.Header.Set("X-Auth-Nonce", string(nonce))
	req.Header.Set("X-Auth-KeyHash-Base64", base64.StdEncoding.EncodeToString(keyHash.Bytes()))

	// Test defaultAuth
	err = defaultAuth(req, db)
	if err != nil {
		t.Fatalf("expected defaultAuth to succeed, got error: %v", err)
	}
}

func TestDefaultAuth_MissingHeaders(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	tests := []struct {
		name        string
		token       string
		nonce       string
		keyHash     string
		expectedErr string
	}{
		{
			name:        "missing all headers",
			token:       "",
			nonce:       "",
			keyHash:     "",
			expectedErr: "missing authentication headers",
		},
		{
			name:        "missing token",
			token:       "",
			nonce:       "nonce",
			keyHash:     "hash",
			expectedErr: "missing authentication headers",
		},
		{
			name:        "missing nonce",
			token:       "token",
			nonce:       "",
			keyHash:     "hash",
			expectedErr: "missing authentication headers",
		},
		{
			name:        "missing keyHash",
			token:       "token",
			nonce:       "nonce",
			keyHash:     "",
			expectedErr: "missing authentication headers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/data", nil)
			if tt.token != "" {
				req.Header.Set("X-Auth-Token", tt.token)
			}
			if tt.nonce != "" {
				req.Header.Set("X-Auth-Nonce", tt.nonce)
			}
			if tt.keyHash != "" {
				req.Header.Set("X-Auth-KeyHash-Base64", tt.keyHash)
			}

			err := defaultAuth(req, db)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if err.Error() != tt.expectedErr {
				t.Fatalf("expected error %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestDefaultAuth_InvalidBase64KeyHash(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", "token")
	req.Header.Set("X-Auth-Nonce", "nonce")
	req.Header.Set("X-Auth-KeyHash-Base64", "not-valid-base64!!!")

	err := defaultAuth(req, db)
	if err == nil {
		t.Fatalf("expected error for invalid base64, got nil")
	}
	if err.Error()[:len("invalid X-Auth-KeyHash-Base64")] != "invalid X-Auth-KeyHash-Base64" {
		t.Fatalf("expected invalid base64 error, got: %v", err)
	}
}

func TestDefaultAuth_KeyNotFound(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	// Use a non-existent key hash
	nonExistentHash := cryptHash.HashBytes([]byte("non-existent"))

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", "token")
	req.Header.Set("X-Auth-Nonce", "nonce")
	req.Header.Set("X-Auth-KeyHash-Base64", base64.StdEncoding.EncodeToString(nonExistentHash.Bytes()))

	err := defaultAuth(req, db)
	if err == nil {
		t.Fatalf("expected error for non-existent key, got nil")
	}
	if err.Error()[:len("failed to retrieve auth key")] != "failed to retrieve auth key" {
		t.Fatalf("expected retrieve error, got: %v", err)
	}
}

func TestDefaultAuth_ExpiredKey(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// Create expired BrowserKey
	browserKey := browserCrypt.BrowserKey{
		Key:       key,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // expired 1 hour ago
		Nonce:     nil,
	}

	keyData, err := json.Marshal(browserKey)
	if err != nil {
		t.Fatalf("failed to marshal browser key: %v", err)
	}

	keyHash, err := db.StoreData(context.Background(), keyData, ouroboros.StoreOptions{})
	if err != nil {
		t.Fatalf("failed to store key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", "token")
	req.Header.Set("X-Auth-Nonce", "nonce")
	req.Header.Set("X-Auth-KeyHash-Base64", base64.StdEncoding.EncodeToString(keyHash.Bytes()))

	err = defaultAuth(req, db)
	if err == nil {
		t.Fatalf("expected error for expired key, got nil")
	}
	if err.Error() != "authentication key expired" {
		t.Fatalf("expected expired key error, got: %v", err)
	}
}

func TestDefaultAuth_InvalidTokenDecryption(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	browserKey := browserCrypt.BrowserKey{
		Key:       key,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Nonce:     nil,
	}

	keyData, err := json.Marshal(browserKey)
	if err != nil {
		t.Fatalf("failed to marshal browser key: %v", err)
	}

	keyHash, err := db.StoreData(context.Background(), keyData, ouroboros.StoreOptions{})
	if err != nil {
		t.Fatalf("failed to store key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", "invalid-ciphertext")
	req.Header.Set("X-Auth-Nonce", "123456789012")
	req.Header.Set("X-Auth-KeyHash-Base64", base64.StdEncoding.EncodeToString(keyHash.Bytes()))

	err = defaultAuth(req, db)
	if err == nil {
		t.Fatalf("expected error for invalid token, got nil")
	}
	if err.Error()[:len("failed to decrypt token")] != "failed to decrypt token" {
		t.Fatalf("expected decrypt error, got: %v", err)
	}
}

func TestDefaultAuth_UnexpectedTokenContent(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	nonce := []byte("123456789012")
	plaintext := []byte("wrong:content") // doesn't start with "auth:"

	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("failed to create GCM: %v", err)
	}

	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	browserKey := browserCrypt.BrowserKey{
		Key:       key,
		ExpiresAt: time.Now().Add(1 * time.Hour),
		Nonce:     nil,
	}

	keyData, err := json.Marshal(browserKey)
	if err != nil {
		t.Fatalf("failed to marshal browser key: %v", err)
	}

	keyHash, err := db.StoreData(context.Background(), keyData, ouroboros.StoreOptions{})
	if err != nil {
		t.Fatalf("failed to store key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("X-Auth-Token", string(ciphertext))
	req.Header.Set("X-Auth-Nonce", string(nonce))
	req.Header.Set("X-Auth-KeyHash-Base64", base64.StdEncoding.EncodeToString(keyHash.Bytes()))

	err = defaultAuth(req, db)
	if err == nil {
		t.Fatalf("expected error for unexpected token content, got nil")
	}
	if err.Error() != "unexpected token content" {
		t.Fatalf("expected unexpected content error, got: %v", err)
	}
}

// Test authProcess function

func TestAuthProcess_Success(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	// Create OTK
	otkKey := make([]byte, 32)
	for i := range otkKey {
		otkKey[i] = byte(i + 1)
	}
	otkNonce := []byte("123456789012")

	sha512Sum := sha512.Sum512(otkKey)
	userHash := fmt.Sprintf("%x", sha512Sum)

	server.authStore.OTK[userHash] = browserCrypt.BrowserKey{
		Key:       otkKey,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Nonce:     otkNonce,
	}

	// Prepare user's actual auth key
	userAuthKey := []byte("user-auth-key-32-bytes-long!!")

	// Encrypt the user's auth key with OTK
	block, err := aes.NewCipher(otkKey)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("failed to create GCM: %v", err)
	}

	encryptedData := aesgcm.Seal(nil, otkNonce, userAuthKey, nil)

	// Create request
	reqData := authProcessReq{
		EncryptedData: string(encryptedData),
		SHA512:        userHash,
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d, body: %s", http.StatusOK, w.Code, w.Body.String())
	}

	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if response["status"] != "authenticated" {
		t.Fatalf("expected status 'authenticated', got %q", response["status"])
	}

	if response["keyKvHash"] == "" {
		t.Fatalf("expected keyKvHash in response")
	}

	// Verify OTK was deleted (one-time use)
	if _, exists := server.authStore.OTK[userHash]; exists {
		t.Fatalf("expected OTK to be deleted after use")
	}

	// Verify the key was stored in the database
	keyHash, err := cryptHash.HashHexadecimal(response["keyKvHash"])
	if err != nil {
		t.Fatalf("failed to parse keyKvHash: %v", err)
	}

	storedData, err := db.GetData(context.Background(), keyHash)
	if err != nil {
		t.Fatalf("failed to retrieve stored key: %v", err)
	}

	var storedBrowserKey browserCrypt.BrowserKey
	if err := json.Unmarshal(storedData.Content, &storedBrowserKey); err != nil {
		t.Fatalf("failed to unmarshal stored key: %v", err)
	}

	if !bytes.Equal(storedBrowserKey.Key, userAuthKey) {
		t.Fatalf("stored key doesn't match original auth key")
	}
}

func TestAuthProcess_MissingSHA512(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	reqData := authProcessReq{
		EncryptedData: "some-data",
		SHA512:        "",
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("missing sha512/hash")) {
		t.Fatalf("expected 'missing sha512/hash' error message, got: %s", w.Body.String())
	}
}

func TestAuthProcess_InvalidOTK(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	reqData := authProcessReq{
		EncryptedData: "some-data",
		SHA512:        "non-existent-hash",
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("invalid or expired one-time key")) {
		t.Fatalf("expected 'invalid or expired one-time key' error, got: %s", w.Body.String())
	}
}

func TestAuthProcess_ExpiredOTK(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	otkKey := make([]byte, 32)
	for i := range otkKey {
		otkKey[i] = byte(i)
	}

	sha512Sum := sha512.Sum512(otkKey)
	userHash := fmt.Sprintf("%x", sha512Sum)

	// Create expired OTK
	server.authStore.OTK[userHash] = browserCrypt.BrowserKey{
		Key:       otkKey,
		ExpiresAt: time.Now().Add(-1 * time.Minute), // expired
		Nonce:     []byte("nonce"),
	}

	reqData := authProcessReq{
		EncryptedData: "some-data",
		SHA512:        userHash,
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("expired one-time key")) {
		t.Fatalf("expected 'expired one-time key' error, got: %s", w.Body.String())
	}

	// Verify expired OTK was deleted
	if _, exists := server.authStore.OTK[userHash]; exists {
		t.Fatalf("expected expired OTK to be deleted")
	}
}

func TestAuthProcess_MissingEncryptedData(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	otkKey := make([]byte, 32)
	for i := range otkKey {
		otkKey[i] = byte(i)
	}

	sha512Sum := sha512.Sum512(otkKey)
	userHash := fmt.Sprintf("%x", sha512Sum)

	server.authStore.OTK[userHash] = browserCrypt.BrowserKey{
		Key:       otkKey,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Nonce:     []byte("nonce"),
	}

	reqData := authProcessReq{
		EncryptedData: "",
		SHA512:        userHash,
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if !bytes.Contains(w.Body.Bytes(), []byte("missing data")) {
		t.Fatalf("expected 'missing data' error, got: %s", w.Body.String())
	}
}

func TestAuthProcess_HashKeyMismatch(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	otkKey := make([]byte, 32)
	for i := range otkKey {
		otkKey[i] = byte(i)
	}

	// Create a different hash that doesn't match the key
	wrongKey := make([]byte, 32)
	for i := range wrongKey {
		wrongKey[i] = byte(i + 100)
	}
	sha512Sum := sha512.Sum512(wrongKey)
	userHash := fmt.Sprintf("%x", sha512Sum)

	server.authStore.OTK[userHash] = browserCrypt.BrowserKey{
		Key:       otkKey, // different key
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Nonce:     []byte("nonce"),
	}

	reqData := authProcessReq{
		EncryptedData: "some-data",
		SHA512:        userHash,
	}

	w := httptest.NewRecorder()
	server.authProcess(context.Background(), w, reqData)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// The OTK should have been deleted even on failure (one-time use enforced)
	if _, exists := server.authStore.OTK[userHash]; exists {
		t.Fatalf("expected OTK to be deleted after verification attempt")
	}
}

func TestAuthProcess_InvalidEncryptionData(t *testing.T) {
	db, cleanup := newTestDB(t)
	t.Cleanup(cleanup)

	server := &Server{
		db:        db,
		log:       testLogger(),
		authStore: browserCrypt.AuthStore{OTK: make(map[string]browserCrypt.BrowserKey)},
	}

	otkKey := make([]byte, 32)
	for i := range otkKey {
		otkKey[i] = byte(i)
	}
	otkNonce := []byte("123456789012")

	sha512Sum := sha512.Sum512(otkKey)
	userHash := fmt.Sprintf("%x", sha512Sum)

	server.authStore.OTK[userHash] = browserCrypt.BrowserKey{
		Key:       otkKey,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Nonce:     otkNonce,
	}

	reqData := authProcessReq{
		EncryptedData: "invalid-encrypted-data",
		SHA512:        userHash,
	}

	w := httptest.NewRecorder()

	// This should panic due to invalid decryption, but we expect it to be handled
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for invalid encrypted data")
		}
	}()

	server.authProcess(context.Background(), w, reqData)
}
