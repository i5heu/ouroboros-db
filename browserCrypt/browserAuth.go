package browserCrypt

import (
	"crypto/sha512"
	"fmt"
	"time"
)

type AuthStore struct {
	OTK map[string]BrowserKey
}

type BrowserKey struct {
	Key       []byte
	ExpiresAt time.Time
	Nonce     []byte
}

func CreateBrowserOTK(as AuthStore) (key []byte, nonce []byte, err error) { // PHC
	key, nonce, err = createOneTimeKeyForAuthentication()
	if err != nil {
		return nil, nil, err
	}

	bk := BrowserKey{
		Key:       key,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Nonce:     nonce,
	}

	// add to auth store
	sha512Sum := sha512.Sum512(bk.Key)
	userHash := fmt.Sprintf("%x", sha512Sum)
	as.OTK[userHash] = bk

	return key, nonce, nil
}
