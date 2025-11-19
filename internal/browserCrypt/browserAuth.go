package browserCrypt

import (
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
