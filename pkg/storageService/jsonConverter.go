package storageService

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
)

func (item EventChainItem) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		Level             int64    `json:"level"`
		ContentMetaHash   string   `json:"contentMetaHash"`
		ContentHashes     []string `json:"contentHashes"`
		MetadataHashes    []string `json:"metadataHashes"`
		HashOfParentEvent string   `json:"hashOfParentEvent"`
		HashOfSourceEvent string   `json:"hashOfSourceEvent"`
		Temporary         bool     `json:"temporary"`
	}{
		Level:             item.Level,
		ContentMetaHash:   hex.EncodeToString(item.ContentMetaHash[:]),
		ContentHashes:     convertHashArrayToStrings(item.ContentHashes),
		MetadataHashes:    convertHashArrayToStrings(item.MetadataHashes),
		HashOfParentEvent: hex.EncodeToString(item.HashOfParentEvent[:]),
		HashOfSourceEvent: hex.EncodeToString(item.HashOfSourceEvent[:]),
		Temporary:         item.Temporary,
	}, "", "    ") // The "" can be any prefix, and "    " sets the indent to four spaces
}

func convertHashArrayToStrings(hashes [][64]byte) []string {
	strs := make([]string, len(hashes))
	for i, hash := range hashes {
		strs[i] = hex.EncodeToString(hash[:])
	}
	return strs
}

func generateRandomHash() [64]byte {
	var hash [64]byte
	_, err := rand.Read(hash[:])
	if err != nil {
		panic(err)
	}
	return hash
}

func generateRandomHashes(n int) [][64]byte {
	hashes := make([][64]byte, n)
	for i := range hashes {
		hashes[i] = generateRandomHash()
	}
	return hashes
}
