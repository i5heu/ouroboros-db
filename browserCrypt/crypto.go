package browserCrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
)

func DecryptWithAESGCM(key, nonce, ciphertext []byte) ([]byte, error) { // PHC
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// Will create a one-time key file that has the key for AES-256-GCM encryption in it.
func CreateOneTimeKeyForAuthentication() (key []byte, nonce []byte, err error) { // PHC
	// Generate a new random key and nonce for AES-256-GCM.
	key = make([]byte, 32)
	_, err = rand.Read(key)
	if err != nil {
		return nil, nil, err
	}

	nonce = make([]byte, 12)
	_, err = rand.Read(nonce)
	if err != nil {
		return nil, nil, err
	}
	return key, nonce, nil
}
