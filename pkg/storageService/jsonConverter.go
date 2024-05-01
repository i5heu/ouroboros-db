package storageService

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

func (indexItem RootEventsIndex) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		Key        string `json:"key"`
		KeyOfEvent string `json:"keyOfEvent"`
	}{
		Key:        string(indexItem.Key),
		KeyOfEvent: string(indexItem.KeyOfEvent),
	}, "", "    ")
}

func (item Event) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		Key               string   `json:"key"`
		EventHash         string   `json:"eventHash"`
		Level             int64    `json:"level"`
		ContentHashes     []string `json:"contentHashes"`
		MetadataHashes    []string `json:"metadataHashes"`
		HashOfParentEvent string   `json:"hashOfParentEvent"`
		HashOfSourceEvent string   `json:"hashOfSourceEvent"`
		Temporary         bool     `json:"temporary"`
	}{
		Key:               string(item.Key),
		Level:             item.Level,
		EventHash:         hex.EncodeToString(item.EventHash[:]),
		ContentHashes:     convertHashArrayToStrings(item.ContentHashes),
		MetadataHashes:    convertHashArrayToStrings(item.MetadataHashes),
		HashOfParentEvent: hex.EncodeToString(item.HashOfParentEvent[:]),
		HashOfSourceEvent: hex.EncodeToString(item.HashOfRootEvent[:]),
		Temporary:         item.Temporary,
	}, "", "    ")
}

func convertHashArrayToStrings(hashes [][64]byte) []string {
	strs := make([]string, len(hashes))
	for i, hash := range hashes {
		strs[i] = hex.EncodeToString(hash[:])
	}
	return strs
}

func (item *Event) PrettyPrint() {
	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		fmt.Println("Error marshalling EventChainItem to JSON:", err)
		return
	}

	fmt.Println(string(jsonBytes))
}
