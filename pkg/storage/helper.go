package storage

import (
	"encoding/hex"

	"github.com/i5heu/ouroboros-db/pkg/types"
)

func GenerateKeyFromPrefixAndHash(prefix string, hash types.Hash) []byte {
	return append([]byte(prefix), []byte(hex.EncodeToString(hash[:]))...)
}
