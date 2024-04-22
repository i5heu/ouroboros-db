package storageService

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
)

func (item EventChainItem) MarshalJSON() ([]byte, error) {
	type Alias EventChainItem
	return json.Marshal(&struct {
		Level             int64    `json:"level"`
		ContentMetaHash   string   `json:"contentMetaHash"`
		ContentHashes     []string `json:"contentHashes"`
		MetadataHashes    []string `json:"metadataHashes"`
		HashOfParentEvent string   `json:"hashOfParentEvent"`
		HashOfSourceEvent string   `json:"hashOfSourceEvent"`
		Temporary         bool     `json:"temporary"`
		*Alias
	}{
		Level:             item.Level,
		ContentMetaHash:   base64.StdEncoding.EncodeToString(item.ContentMetaHash[:]),
		ContentHashes:     convertHashArrayToStrings(item.ContentHashes),
		MetadataHashes:    convertHashArrayToStrings(item.MetadataHashes),
		HashOfParentEvent: base64.StdEncoding.EncodeToString(item.HashOfParentEvent[:]),
		HashOfSourceEvent: base64.StdEncoding.EncodeToString(item.HashOfSourceEvent[:]),
		Temporary:         item.Temporary,
		Alias:             (*Alias)(&item),
	})
}

func convertHashArrayToStrings(hashes [][64]byte) []string {
	strs := make([]string, len(hashes))
	for i, hash := range hashes {
		strs[i] = base64.StdEncoding.EncodeToString(hash[:])
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
