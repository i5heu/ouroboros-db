// storage_test.go
package storage

import (
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"testing"
)

func TestRootEventsIndex_MarshalJSON(t *testing.T) {
	indexItem := RootEventsIndex{
		Title: []byte("hello world 12345678"),
		Hash:  sha512.Sum512([]byte("hello world")),
	}

	expectedJSON := `{
    "key": "hello world 12345678",
    "keyOfEvent": "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f"
}`

	jsonBytes, err := indexItem.MarshalJSON()
	if err != nil {
		t.Fatalf("Expected no error but got %v", err)
	}

	var jsonObject map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonObject)
	if err != nil {
		t.Fatalf("Expected valid JSON but got error %v", err)
	}

	if string(jsonBytes) != expectedJSON {
		t.Errorf("Expected %s but got %s", expectedJSON, string(jsonBytes))
	}
}

func TestEvent_MarshalJSON(t *testing.T) {
	event := Event{
		Key:               []byte("hello world 12345678"),
		EventHash:         sha512.Sum512([]byte("hello world 2")),
		Level:             42,
		ContentHashes:     [][64]byte{sha512.Sum512([]byte("hello world 3")), sha512.Sum512([]byte("hello world 4"))},
		MetadataHashes:    [][64]byte{sha512.Sum512([]byte("hello world 5")), sha512.Sum512([]byte("hello world 6"))},
		HashOfParentEvent: sha512.Sum512([]byte("hello world 7")),
		HashOfRootEvent:   sha512.Sum512([]byte("hello world 8")),
		Temporary:         true,
	}

	jsonBytes, err := event.MarshalJSON()
	if err != nil {
		t.Fatalf("Expected no error but got %v", err)
	}

	var jsonObject map[string]interface{}
	err = json.Unmarshal(jsonBytes, &jsonObject)
	if err != nil {
		t.Fatalf("Expected valid JSON but got error %v", err)
	}

	expectedKey := "hello world 12345678"
	eventHash := sha512.Sum512([]byte("hello world 2"))
	expectedEventHash := hex.EncodeToString(eventHash[:])

	contentHash3 := sha512.Sum512([]byte("hello world 3"))
	contentHash4 := sha512.Sum512([]byte("hello world 4"))
	expectedContentHashes := []string{
		hex.EncodeToString(contentHash3[:]),
		hex.EncodeToString(contentHash4[:]),
	}

	metadataHash5 := sha512.Sum512([]byte("hello world 5"))
	metadataHash6 := sha512.Sum512([]byte("hello world 6"))
	expectedMetadataHashes := []string{
		hex.EncodeToString(metadataHash5[:]),
		hex.EncodeToString(metadataHash6[:]),
	}

	parentHash := sha512.Sum512([]byte("hello world 7"))
	expectedParentHash := hex.EncodeToString(parentHash[:])
	rootHash := sha512.Sum512([]byte("hello world 8"))
	expectedRootHash := hex.EncodeToString(rootHash[:])

	if jsonObject["key"] != expectedKey {
		t.Errorf("Expected key '%v' but got '%v'", expectedKey, jsonObject["key"])
	}
	if jsonObject["eventHash"] != expectedEventHash {
		t.Errorf("Expected eventHash '%v' but got '%v'", expectedEventHash, jsonObject["eventHash"])
	}
	if jsonObject["level"] != float64(42) {
		t.Errorf("Expected level 42 but got %v", jsonObject["level"])
	}
	if !equalStringSlices(jsonObject["contentHashes"].([]interface{}), expectedContentHashes) {
		t.Errorf("Expected contentHashes %v but got %v", expectedContentHashes, jsonObject["contentHashes"])
	}
	if !equalStringSlices(jsonObject["metadataHashes"].([]interface{}), expectedMetadataHashes) {
		t.Errorf("Expected metadataHashes %v but got %v", expectedMetadataHashes, jsonObject["metadataHashes"])
	}
	if jsonObject["hashOfParentEvent"] != expectedParentHash {
		t.Errorf("Expected hashOfParentEvent '%v' but got '%v'", expectedParentHash, jsonObject["hashOfParentEvent"])
	}
	if jsonObject["hashOfRootEvent"] != expectedRootHash {
		t.Errorf("Expected hashOfRootEvent '%v' but got '%v'", expectedRootHash, jsonObject["hashOfRootEvent"])
	}
	if jsonObject["temporary"] != true {
		t.Errorf("Expected temporary true but got %v", jsonObject["temporary"])
	}

}

func equalStringSlices(a []interface{}, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestConvertHashArrayToStrings(t *testing.T) {
	hashes := [][64]byte{
		sha512.Sum512([]byte("hello world 1")),
		sha512.Sum512([]byte("hello world 2")),
	}

	expected := []string{
		"51bd89d9fde19c8dd2b089ffb6a6bbabb24ecc463cdbbba63cfb23ee35ba678b8ac1ce2d56428753b320d22c3e402786f9df49ad6271cae4e973a75ab10249c3",
		"c41b117e8e10b6f26db3db583f16802163113469759516eade27027d0e9c8f3a9b9d6338766cb548b9486f9e6ff3b6fc1f4ab46a30611b807900adce10a8da67",
	}

	result := convertHashArrayToStrings(hashes)
	convertedResult := make([]interface{}, len(result))
	for i, v := range result {
		convertedResult[i] = v
	}

	if !equalStringSlices(convertedResult, expected) {
		t.Errorf("Expected %v but got %v", expected, convertedResult)
	}

}

func TestEvent_PrettyPrint(t *testing.T) {
	event := Event{
		Key:               []byte("eventKey"),
		EventHash:         [64]byte{0x12, 0x34, 0x56, 0x78},
		Level:             42,
		ContentHashes:     [][64]byte{{0x98, 0x76}, {0xab, 0xcd}},
		MetadataHashes:    [][64]byte{{0xef, 0x12}, {0x34, 0x56}},
		HashOfParentEvent: [64]byte{0xde, 0xad},
		HashOfRootEvent:   [64]byte{0xbe, 0xef},
		Temporary:         true,
	}

	// We can only print the output visually, this test will just ensure it doesn't panic
	event.PrettyPrint()
}
