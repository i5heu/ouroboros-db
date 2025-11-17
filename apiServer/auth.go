package apiServer

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	cryptHash "github.com/i5heu/ouroboros-crypt/hash"
	ouroboros "github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/browserCrypt"
	browserAuth "github.com/i5heu/ouroboros-db/browserCrypt"
)

type authProcessReq struct {
	EncryptedData string `json:"data"`
	SHA512        string `json:"sha512"`
}

func defaultAuth(req *http.Request, db *ouroboros.OuroborosDB) error { // PHC
	reqToken := req.Header.Get("X-Auth-Token")
	reqNonce := req.Header.Get("X-Auth-Nonce")
	keyKvHash := req.Header.Get("X-Auth-KeyHash-Base64")
	if reqToken == "" || reqNonce == "" || keyKvHash == "" {
		return fmt.Errorf("missing authentication headers")
	}

	// turn keyKvHash into cryptHash.Hash
	decoded, err := base64.StdEncoding.DecodeString(keyKvHash)
	if err != nil {
		return fmt.Errorf("invalid X-Auth-KeyHash-Base64: %w", err)
	}

	// Copy the decoded bytes into a Hash
	var keyHash cryptHash.Hash
	if len(decoded) != len(keyHash) {
		return fmt.Errorf("invalid hash length: expected %d bytes, got %d", len(keyHash), len(decoded))
	}
	copy(keyHash[:], decoded)

	keyData, err := db.GetData(req.Context(), keyHash)
	if err != nil {
		return fmt.Errorf("failed to retrieve auth key: %w", err)
	}

	// unmarshal keyData into BrowserKey
	var browserKey browserCrypt.BrowserKey
	if err := json.Unmarshal(keyData.Content, &browserKey); err != nil {
		return fmt.Errorf("failed to unmarshal auth key: %w", err)
	}

	// check if browserKey is expired
	if time.Since(browserKey.ExpiresAt) > 0 {
		// TODO: delete expired key from db
		return fmt.Errorf("authentication key expired")
	}

	// check if reqToken matches browserKey.Key using AES-256-GCM
	plaintext, err := browserAuth.DecryptWithAESGCM(browserKey.Key, []byte(reqNonce), []byte(reqToken))
	if err != nil {
		return fmt.Errorf("failed to decrypt token: %w", err)
	}

	// check if plaintext starts with "auth:"
	if !strings.HasPrefix(string(plaintext), "auth:") {
		return fmt.Errorf("unexpected token content")
	}

	return nil
}

func (s *Server) authProcess(ctx context.Context, w http.ResponseWriter, req authProcessReq) { // PH

	var (
		userHash string
	)

	// support either "sha512"
	userHash = strings.TrimSpace(req.SHA512)
	if userHash == "" {
		http.Error(w, "missing sha512/hash", http.StatusBadRequest)
		return
	}

	// check in the auth store for hash
	otk, ok := s.authStore.OTK[userHash]
	if !ok {
		http.Error(w, "invalid or expired one-time key", http.StatusUnauthorized)
		return
	}

	// if otk expired, return error
	if time.Since(otk.ExpiresAt) > 0 {
		delete(s.authStore.OTK, userHash)
		http.Error(w, "expired one-time key", http.StatusUnauthorized)
		return
	}

	if strings.TrimSpace(req.EncryptedData) == "" {
		http.Error(w, "missing data", http.StatusBadRequest)
		return
	}

	// hash is valid, delete it to enforce one-time use
	delete(s.authStore.OTK, userHash)

	// check if hash and key match
	sha512Sum := sha512.Sum512(otk.Key)
	if fmt.Sprintf("%x", sha512Sum) != userHash {
		http.Error(w, "invalid one-time key", http.StatusUnauthorized)
		return
	}

	// decrypt the userEncryptedData and store the encrypted key in AuthenticatedKeys

	block, err := aes.NewCipher(otk.Key)
	if err != nil {
		panic(err.Error())
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		panic(err.Error())
	}

	plaintext, err := aesgcm.Open(nil, otk.Nonce, []byte(req.EncryptedData), nil)
	if err != nil {
		panic(err.Error())
	}

	newAuth := browserAuth.BrowserKey{
		Key:       plaintext,
		ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
		Nonce:     nil,
	}

	// prepare JSON for storing the authenticated key
	authJson, err := json.Marshal(newAuth)
	if err != nil {
		http.Error(w, "failed to marshal authenticated key", http.StatusInternalServerError)
		return
	}

	// store the authenticated key in oukv
	keyKvHash, err := s.db.StoreData(ctx, authJson, ouroboros.StoreOptions{
		Parent: cryptHash.HashBytes([]byte("StoreAuthenticatedKey")),
	})
	if err != nil {
		http.Error(w, "failed to store authenticated key", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "authenticated", "keyKvHash": keyKvHash.String()})
}
