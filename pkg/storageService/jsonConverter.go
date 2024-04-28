package storageService

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func (indexItem EventChains) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		Key   string `json:"key"`
		Title string `json:"title"`
		Level int64  `json:"level"`
	}{
		Key:   hex.EncodeToString(indexItem.Key),
		Title: indexItem.Title,
		Level: indexItem.Level,
	}, "", "    ") // The "" can be any prefix, and "    " sets the indent to four spaces
}

func (item EventChainItem) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		Key               string   `json:"key"`
		Level             int64    `json:"level"`
		ContentMetaHash   string   `json:"contentMetaHash"`
		ContentHashes     []string `json:"contentHashes"`
		MetadataHashes    []string `json:"metadataHashes"`
		HashOfParentEvent string   `json:"hashOfParentEvent"`
		HashOfSourceEvent string   `json:"hashOfSourceEvent"`
		Temporary         bool     `json:"temporary"`
	}{
		Key:               string(item.Key),
		Level:             item.Level,
		ContentMetaHash:   hex.EncodeToString(item.DetailsMetaHash[:]),
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

func PrettyPrintEventChainItem(item EventChainItem) {
	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		fmt.Println("Error marshalling EventChainItem to JSON:", err)
		return
	}

	fmt.Println(string(jsonBytes))
}
