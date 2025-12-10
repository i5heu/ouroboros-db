package cas

import (
	"bytes"
	"testing"

	_ "github.com/i5heu/ouroboros-db/internal/testutil"
	"github.com/i5heu/ouroboros-db/pkg/api"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// TestData is a local struct to hold test data, similar to Data in ouroboroskv
type TestData struct {
	Key            hash.Hash
	Content        []byte
	Meta           []byte
	Parent         hash.Hash
	Children       []hash.Hash
	RSDataSlices   uint8
	RSParitySlices uint8
	Aliases        []hash.Hash
	Created        int64
}

func createTestData() TestData {
	return TestData{
		Meta:           []byte("Test metadata"),
		Content:        []byte("This is test content for the encryption pipeline. It should be long enough to test chunking functionality."),
		Parent:         hash.HashString("parent-key"),
		Children:       []hash.Hash{hash.HashString("child1"), hash.HashString("child2")},
		RSDataSlices:   3,
		RSParitySlices: 2,
		Key:            hash.HashString("test-key"),
	}
}

func TestEncodePayload(t *testing.T) {
	c := crypt.New()
	testData := createTestData()

	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("EncodePayload failed: %v", err)
	}

	// Verify chunks were created
	if len(slices) == 0 {
		t.Error("Expected content slices to be created, but got none")
	}

	if len(chunkHashes) == 0 {
		t.Error("Expected chunk hashes to be created, but got none")
	}

	// Verify Reed-Solomon settings
	totalSlices := testData.RSDataSlices + testData.RSParitySlices
	for i, chunk := range slices {
		if chunk.RSDataSlices != testData.RSDataSlices {
			t.Errorf("Chunk %d: expected %d Reed-Solomon slices, got %d", i, testData.RSDataSlices, chunk.RSDataSlices)
		}
		if chunk.RSParitySlices != testData.RSParitySlices {
			t.Errorf("Chunk %d: expected %d Reed-Solomon parity slices, got %d", i, testData.RSParitySlices, chunk.RSParitySlices)
		}
		if chunk.RSSliceIndex >= totalSlices {
			t.Errorf("Chunk %d: Reed-Solomon index %d is out of range (max %d)", i, chunk.RSSliceIndex, totalSlices-1)
		}
	}
}

func TestEncodePayloadEmptyContent(t *testing.T) {
	c := crypt.New()
	testData := createTestData()
	testData.Content = []byte{}

	slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
	if err != nil {
		t.Fatalf("EncodePayload with empty content failed: %v", err)
	}

	if len(slices) != 0 {
		t.Errorf("Expected no content slices, got %d", len(slices))
	}

	if len(chunkHashes) != 0 {
		t.Errorf("Expected no chunk hashes, got %d", len(chunkHashes))
	}
}

func TestChunker(t *testing.T) {
	testData := createTestData()

	chunks, err := chunkerFunc(testData.Content)
	if err != nil {
		t.Fatalf("chunker failed: %v", err)
	}

	if len(chunks) == 0 {
		t.Error("Expected at least one chunk")
	}

	// Verify that concatenating chunks gives back original content
	var reconstructed bytes.Buffer
	for _, chunk := range chunks {
		reconstructed.Write(chunk)
	}

	if !bytes.Equal(reconstructed.Bytes(), testData.Content) {
		t.Error("Reconstructed content doesn't match original")
	}
}

func TestChunkerEmptyContent(t *testing.T) {
	testData := createTestData()
	testData.Content = []byte{}
	chunks, err := chunkerFunc(testData.Content)
	if err != nil {
		t.Fatalf("chunker with empty content failed: %v", err)
	}

	if len(chunks) != 0 {
		t.Errorf("Expected 0 chunks for empty content, got %d", len(chunks))
	}
}

func TestCompressWithZstd(t *testing.T) {
	testData := []byte("This is test data for Zstd compression. It should compress well if it's repetitive. " +
		"This is test data for Zstd compression. It should compress well if it's repetitive.")

	compressed, err := compressWithZstd(testData)
	if err != nil {
		t.Fatalf("compressWithZstd failed: %v", err)
	}

	if len(compressed) == 0 {
		t.Error("Expected compressed data to have non-zero length")
	}

	// For repetitive data, compression should reduce size
	// Note: This might not always be true for very small data
	if len(compressed) >= len(testData) {
		t.Logf("Warning: Compressed size (%d) is not smaller than original (%d)", len(compressed), len(testData))
	}
}

func TestCompressWithZstdEmptyData(t *testing.T) {
	compressed, err := compressWithZstd([]byte{})
	if err != nil {
		t.Fatalf("compressWithZstd with empty data failed: %v", err)
	}

	// Zstd returns an empty slice for empty input
	if len(compressed) != 0 {
		t.Errorf("Expected compressed empty data to have zero length, got %d", len(compressed))
	}
}

func TestReedSolomonSplitter(t *testing.T) {
	c := crypt.New()
	testData := createTestData()
	testContent := []byte("test content for reed solomon")

	compressedChunk, err := compressWithZstd(testContent)
	if err != nil {
		t.Fatalf("Failed to compress test content: %v", err)
	}

	compressedChunks := [][]byte{compressedChunk}
	chunkHashes := []hash.Hash{hash.HashBytes(testContent)}

	chunks, err := splitIntoRSSlicesAndEncrypt(testData.RSDataSlices, testData.RSParitySlices, compressedChunks, chunkHashes, c)
	if err != nil {
		t.Fatalf("splitIntoRSSlices failed: %v", err)
	}

	expectedTotalSlices := int(testData.RSDataSlices + testData.RSParitySlices)
	if len(chunks) != expectedTotalSlices {
		t.Errorf("Expected %d chunks, got %d", expectedTotalSlices, len(chunks))
	}

	// Verify chunk properties
	for i, chunk := range chunks {
		if chunk.ChunkHash != chunkHashes[0] {
			t.Errorf("Chunk %d: wrong chunk hash", i)
		}
		if chunk.RSDataSlices != testData.RSDataSlices {
			t.Errorf("Chunk %d: wrong Reed-Solomon slices count", i)
		}
		if chunk.RSParitySlices != testData.RSParitySlices {
			t.Errorf("Chunk %d: wrong Reed-Solomon parity slices count", i)
		}
		if chunk.RSSliceIndex != uint8(i) {
			t.Errorf("Chunk %d: expected index %d, got %d", i, i, chunk.RSSliceIndex)
		}
		if len(chunk.Payload) == 0 {
			t.Errorf("Chunk %d: empty content", i)
		}
		if len(chunk.EncapsulatedKey) == 0 {
			t.Errorf("Chunk %d: empty encapsulated key", i)
		}
		if len(chunk.Nonce) == 0 {
			t.Errorf("Chunk %d: empty nonce", i)
		}
	}
}

func TestReedSolomonSplitterInvalidSliceConfig(t *testing.T) {
	c := crypt.New()
	testData := createTestData()
	testData.RSDataSlices = 0 // Invalid configuration
	testData.RSParitySlices = 1

	testContent := []byte("test content")
	compressedChunk, err := compressWithZstd(testContent)
	if err != nil {
		t.Fatalf("Failed to compress test content: %v", err)
	}

	compressedChunks := [][]byte{compressedChunk}
	chunkHashes := []hash.Hash{hash.HashBytes(testContent)}

	_, err = splitIntoRSSlicesAndEncrypt(testData.RSDataSlices, testData.RSParitySlices, compressedChunks, chunkHashes, c)
	if err == nil {
		t.Error("Expected error for invalid Reed-Solomon configuration, but got none")
	}
}

func TestEncodeDataPipelineIntegration(t *testing.T) {
	c := crypt.New()

	// Test with various content sizes
	testCases := []struct {
		name        string
		contentSize int
	}{
		{"small", 10},
		{"medium", 1000},
		{"large", 10000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			content := make([]byte, tc.contentSize)
			for i := range content {
				content[i] = byte(i % 256)
			}

			testData := TestData{
				Key:            hash.HashString("test-key-" + tc.name),
				Meta:           []byte("metadata-" + tc.name),
				Content:        content,
				Parent:         hash.HashString("parent"),
				Children:       []hash.Hash{},
				RSDataSlices:   2,
				RSParitySlices: 1,
			}

			slices, chunkHashes, err := EncodePayload(testData.Content, testData.RSDataSlices, testData.RSParitySlices, c)
			if err != nil {
				t.Fatalf("EncodePayload failed for %s: %v", tc.name, err)
			}

			// Verify result structure
			if len(slices) == 0 {
				t.Errorf("No chunks created for %s", tc.name)
			}

			// Verify all chunks have consistent Reed-Solomon settings
			expectedTotal := testData.RSDataSlices + testData.RSParitySlices
			chunksByGroup := make(map[hash.Hash][]api.SealedSlice)

			for _, chunk := range slices {
				chunksByGroup[chunk.ChunkHash] = append(chunksByGroup[chunk.ChunkHash], chunk)
			}

			for chunkHash, chunks := range chunksByGroup {
				if len(chunks) != int(expectedTotal) {
					t.Errorf("Chunk group %v: expected %d slices, got %d", chunkHash, expectedTotal, len(chunks))
				}
			}

			decodedContent, _, _, err := ReconstructPayload(slices, chunkHashes, c)
			if err != nil {
				t.Fatalf("ReconstructPayload failed for %s: %v", tc.name, err)
			}

			if !bytes.Equal(decodedContent, testData.Content) {
				t.Errorf("Decoded content mismatch for %s", tc.name)
			}
		})
	}
}
