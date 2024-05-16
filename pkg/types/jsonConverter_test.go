package types_test

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestRootEventsIndex_MarshalJSON(t *testing.T) {
	indexItem := types.RootEventsIndex{
		Title: []byte("hello world 12345678"),
		Hash:  sha512.Sum512([]byte("hello world")),
	}

	expectedJSON := `{
    "key": "hello world 12345678",
    "keyOfEvent": "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f"
}`

	jsonBytes, err := indexItem.MarshalJSON()
	assert.NoError(t, err)

	var jsonObject map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonObject)
	assert.NoError(t, err)

	assert.JSONEq(t, expectedJSON, string(jsonBytes))
}

func TestEvent_MarshalJSON(t *testing.T) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventHash: sha512.Sum512([]byte("hello world 2")),
			EventType: types.Normal,
			FastMeta:  types.FastMeta{[]byte("fast meta")},
		},
		Level:          42,
		Content:        types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("hello world 3")), DataLength: 10}, {Hash: sha512.Sum512([]byte("hello world 4")), DataLength: 10}},
		Metadata:       types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("hello world 5")), DataLength: 10}, {Hash: sha512.Sum512([]byte("hello world 6")), DataLength: 10}},
		ParentEvent:    sha512.Sum512([]byte("hello world 7")),
		RootEvent:      sha512.Sum512([]byte("hello world 8")),
		Temporary:      types.Binary(true),
		FullTextSearch: types.Binary(true),
	}

	jsonBytes, err := event.MarshalJSON()
	assert.NoError(t, err)

	var jsonObject map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonObject)
	assert.NoError(t, err)

	expectedEventHash := hex.EncodeToString(event.EventHash[:])
	expectedFastMetaHash := hex.EncodeToString(event.FastMeta.Hash().Bytes())

	expectedContentHashes := []string{
		hex.EncodeToString(event.Content[0].Hash[:]),
		hex.EncodeToString(event.Content[1].Hash[:]),
	}

	expectedMetadataHashes := []string{
		hex.EncodeToString(event.Metadata[0].Hash[:]),
		hex.EncodeToString(event.Metadata[1].Hash[:]),
	}

	expectedParentHash := hex.EncodeToString(event.ParentEvent[:])
	expectedRootHash := hex.EncodeToString(event.RootEvent[:])

	assert.Equal(t, expectedEventHash, jsonObject["eventHash"])
	assert.Equal(t, "Normal", jsonObject["eventType"])
	assert.Equal(t, float64(42), jsonObject["level"])
	assert.Equal(t, expectedFastMetaHash, jsonObject["fastMetaHash"])
	assert.ElementsMatch(t, expectedContentHashes, convertInterfaceToStringSlice(jsonObject["contentHashes"].([]interface{})))
	assert.ElementsMatch(t, expectedMetadataHashes, convertInterfaceToStringSlice(jsonObject["metadataHashes"].([]interface{})))
	assert.Equal(t, expectedParentHash, jsonObject["hashOfParentEvent"])
	assert.Equal(t, expectedRootHash, jsonObject["hashOfRootEvent"])
	assert.Equal(t, true, jsonObject["temporary"])
	assert.Equal(t, true, jsonObject["fullTextSearch"])
}

func convertInterfaceToStringSlice(interfaces []interface{}) []string {
	strings := make([]string, len(interfaces))
	for i, v := range interfaces {
		strings[i] = v.(string)
	}
	return strings
}

func TestEvent_PrettyPrint(t *testing.T) {
	event := types.Event{
		EventIdentifier: types.EventIdentifier{
			EventHash: sha512.Sum512([]byte("eventKey")),
			EventType: types.Normal,
			FastMeta:  types.FastMeta{[]byte("fast meta")},
		},
		Level:          42,
		Content:        types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("content chunk")), DataLength: 0}},
		Metadata:       types.ChunkMetaCollection{{Hash: sha512.Sum512([]byte("metadata chunk")), DataLength: 0}},
		ParentEvent:    sha512.Sum512([]byte("parentEvent")),
		RootEvent:      sha512.Sum512([]byte("rootEvent")),
		Temporary:      types.Binary(true),
		FullTextSearch: types.Binary(true),
	}

	// This test will only ensure it doesn't panic; visual check needed for actual output
	event.PrettyPrint()
}

func TestEvent_UnmarshalInvalidJSON(t *testing.T) {
	invalidJSON := `{
		"eventHash": "invalidhash",
		"eventType": "Normal",
		"level": 42,
		"fastMetaHash": "invalidhash",
		"metadataHashes": ["invalidhash1", "invalidhash2"],
		"contentHashes": ["invalidhash1", "invalidhash2"],
		"hashOfParentEvent": "invalidhash",
		"hashOfRootEvent": "invalidhash",
		"temporary": true,
		"fullTextSearch": true
	}`

	var unmarshalledEvent types.Event
	err := json.Unmarshal([]byte(invalidJSON), &unmarshalledEvent)
	assert.Error(t, err, "Expected an error for invalid JSON")
}
