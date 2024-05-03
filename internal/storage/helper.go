package storage

import "encoding/hex"

func GenerateKeyFromPrefixAndHash(prefix string, hash [64]byte) []byte {
	return append([]byte(prefix), []byte(hex.EncodeToString(hash[:]))...)
}

func (item *Event) GetParentEventKey() []byte {
	return GenerateKeyFromPrefixAndHash("Event:", item.HashOfParentEvent)
}
