package types

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
		Key:        string(indexItem.Title),
		KeyOfEvent: hex.EncodeToString(indexItem.Hash[:]),
	}, "", "    ")
}

func (item Event) MarshalJSON() ([]byte, error) {
	return json.MarshalIndent(&struct {
		EventHash         string   `json:"eventHash"`
		EventType         string   `json:"eventType"`
		Level             int64    `json:"level"`
		FastMetaHash      string   `json:"fastMetaHash"`
		MetadataHashes    []string `json:"metadataHashes"`
		ContentHashes     []string `json:"contentHashes"`
		HashOfParentEvent string   `json:"hashOfParentEvent"`
		HashOfRootEvent   string   `json:"hashOfRootEvent"`
		Temporary         bool     `json:"temporary"`
		FullTextSearch    bool     `json:"fullTextSearch"`
	}{
		EventHash:         hex.EncodeToString(item.EventHash[:]),
		EventType:         item.EventType.String(),
		Level:             int64(item.Level),
		FastMetaHash:      hex.EncodeToString(item.FastMeta.Hash().Bytes()),
		MetadataHashes:    convertChunkMetaCollectionToStrings(item.Metadata),
		ContentHashes:     convertChunkMetaCollectionToStrings(item.Content),
		HashOfParentEvent: hex.EncodeToString(item.ParentEvent[:]),
		HashOfRootEvent:   hex.EncodeToString(item.RootEvent[:]),
		Temporary:         bool(item.Temporary),
		FullTextSearch:    bool(item.FullTextSearch),
	}, "", "    ")
}

func convertChunkMetaCollectionToStrings(chunks ChunkMetaCollection) []string {
	strs := make([]string, len(chunks))
	for i, chunk := range chunks {
		strs[i] = hex.EncodeToString(chunk.Hash[:])
	}
	return strs
}

func (item *Event) PrettyPrint() {
	jsonBytes, err := item.MarshalJSON()
	if err != nil {
		fmt.Println("Error marshalling Event to JSON:", err)
		return
	}

	fmt.Println(string(jsonBytes))
}
