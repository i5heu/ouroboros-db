package types

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"strconv"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db/internal/protoFastMeta"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/proto"
)

func TestBinary_Bytes(t *testing.T) {
	trueBinary := Binary(true)
	falseBinary := Binary(false)

	if !bytes.Equal(trueBinary.Bytes(), []byte(TrueStr)) {
		t.Errorf("Expected %s but got %s", TrueStr, trueBinary.Bytes())
	}

	if !bytes.Equal(falseBinary.Bytes(), []byte(FalseStr)) {
		t.Errorf("Expected %s but got %s", FalseStr, falseBinary.Bytes())
	}
}

func TestBinary_String(t *testing.T) {
	trueBinary := Binary(true)
	falseBinary := Binary(false)

	if trueBinary.String() != TrueStr {
		t.Errorf("Expected %s but got %s", TrueStr, trueBinary.String())
	}

	if falseBinary.String() != FalseStr {
		t.Errorf("Expected %s but got %s", FalseStr, falseBinary.String())
	}
}

func TestTitle_Bytes(t *testing.T) {
	title := Title("EventTitle")
	expectedBytes := []byte("EventTitle")

	if !bytes.Equal(title.Bytes(), expectedBytes) {
		t.Errorf("Expected %s but got %s", expectedBytes, title.Bytes())
	}
}

func TestLevel_Bytes(t *testing.T) {
	level := Level(1234567890)
	expectedBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(expectedBytes, uint64(level))

	if !bytes.Equal(level.Bytes(), expectedBytes) {
		t.Errorf("Expected %v but got %v", expectedBytes, level.Bytes())
	}
}

func TestLevel_String(t *testing.T) {
	level := Level(1234567890)
	expectedString := strconv.FormatInt(int64(level), 10)

	if level.String() != expectedString {
		t.Errorf("Expected %s but got %s", expectedString, level.String())
	}
}

func TestLevel_Time(t *testing.T) {
	level := Level(1234567890)
	expectedTime := time.Unix(0, int64(level))

	if !level.Time().Equal(expectedTime) {
		t.Errorf("Expected %v but got %v", expectedTime, level.Time())
	}
}

func TestLevel_SetToNow(t *testing.T) {
	level := Level(0)
	now := time.Now().UnixNano()
	level.SetToNow()

	if int64(level) < now || int64(level) > time.Now().UnixNano() {
		t.Errorf("Expected level to be set to current time, but got %v", level)
	}
}

func TestFastMeta_IsToBig(t *testing.T) {
	meta := FastMeta{
		[]byte("param1"),
		[]byte("param2"),
	}

	if meta.IsToBig() {
		t.Errorf("Expected IsToBig to be false")
	}

	largeParam := make([]byte, FastMetaMaxSize+1)
	meta = append(meta, largeParam)

	if !meta.IsToBig() {
		t.Errorf("Expected IsToBig to be true")
	}
}

func TestFastMeta_Size(t *testing.T) {
	meta := FastMeta{
		[]byte("param1"),
		[]byte("param2"),
	}

	expectedSize := len("param1") + len("param2")

	if meta.Size() != expectedSize {
		t.Errorf("Expected Size to be %d but got %d", expectedSize, meta.Size())
	}
}

func TestFastMeta_SizeLeft(t *testing.T) {
	meta := FastMeta{
		[]byte("param1"),
		[]byte("param2"),
	}

	expectedSizeLeft := FastMetaMaxSize - len("param1") - len("param2")

	if meta.SizeLeft() != expectedSizeLeft {
		t.Errorf("Expected SizeLeft to be %d but got %d", expectedSizeLeft, meta.SizeLeft())
	}
}

func TestFastMetaBytes(t *testing.T) {
	// Create a FastMeta instance with some parameters
	original := FastMeta{
		FastMetaParameter([]byte("param1")),
		FastMetaParameter([]byte("param2")),
		FastMetaParameter([]byte("param3")),
	}

	// Convert FastMeta to bytes
	bytes := original.Bytes()

	// Convert bytes back to FastMeta
	fm := FastMeta{}
	fm.FromBytes(bytes)

	// Assert that the original and reconstructed FastMeta are the same
	assert.Equal(t, original, fm, "The original and reconstructed FastMeta should be equal")
}

func TestFastMetaFromBytes(t *testing.T) {
	// Create a byte array representing FastMeta
	fastMetaProto := protoFastMeta.FastMetaProto{
		Parameters: []*protoFastMeta.FastMetaParameterProto{
			{Parameter: []byte("param1")},
			{Parameter: []byte("param2")},
			{Parameter: []byte("param3")},
		},
	}

	// Marshal the FastMetaProto to bytes
	data, err := proto.Marshal(&fastMetaProto)
	if err != nil {
		t.Fatalf("Failed to marshal proto: %v", err)
	}

	// Convert bytes back to FastMeta
	fm := FastMeta{}
	fm.FromBytes(data)

	// Create the expected FastMeta instance
	expected := FastMeta{
		FastMetaParameter([]byte("param1")),
		FastMetaParameter([]byte("param2")),
		FastMetaParameter([]byte("param3")),
	}

	// Assert that the reconstructed FastMeta is equal to the expected one
	assert.Equal(t, expected, fm, "The reconstructed FastMeta should be equal to the expected FastMeta")
}

func TestFastMeta_Hash(t *testing.T) {
	meta := FastMeta{
		FastMetaParameter([]byte("param1")),
		FastMetaParameter([]byte("param2")),
	}

	hash := meta.Hash()

	// Create expected hash manually
	var buffer bytes.Buffer
	for _, parameter := range meta {
		buffer.Write(parameter[:])
		buffer.WriteString(":")
	}
	expectedHash := sha512.Sum512(buffer.Bytes())

	assert.Equal(t, Hash(expectedHash), hash)
}

func TestFastMeta_Bytes(t *testing.T) {
	meta := FastMeta{
		[]byte("param1"),
		[]byte("param2"),
	}

	bytes := meta.Bytes()

	// Unmarshal bytes back to FastMeta and compare
	var protoMeta FastMeta
	err := protoMeta.FromBytes(bytes)
	assert.NoError(t, err)

	assert.Equal(t, meta, protoMeta)
}

func TestFastMeta_FromBytes(t *testing.T) {
	metaProto := protoFastMeta.FastMetaProto{
		Parameters: []*protoFastMeta.FastMetaParameterProto{
			{Parameter: []byte("param1")},
			{Parameter: []byte("param2")},
			{Parameter: []byte("param3")},
		},
	}

	data, err := proto.Marshal(&metaProto)
	assert.NoError(t, err)

	var meta FastMeta
	err = meta.FromBytes(data)
	assert.NoError(t, err)

	expectedMeta := FastMeta{
		[]byte("param1"),
		[]byte("param2"),
		[]byte("param3"),
	}

	assert.Equal(t, expectedMeta, meta)
}

func TestEventType_Bytes(t *testing.T) {
	tests := []struct {
		eventType     EventType
		expectedBytes []byte
	}{
		{Normal, []byte{0}},
		{Overwrite, []byte{1}},
		{Patch, []byte{2}},
		{Root, []byte{3}},
	}

	for _, test := range tests {
		assert.Equal(t, test.expectedBytes, test.eventType.Bytes())
	}
}

func TestHash_HashFromBytes(t *testing.T) {
	validBytes := make([]byte, 64)
	for i := range validBytes {
		validBytes[i] = byte(i)
	}

	var h Hash
	err := h.HashFromBytes(validBytes)
	assert.NoError(t, err)
	assert.Equal(t, validBytes, h.Bytes())

	invalidBytes := make([]byte, 63)
	err = h.HashFromBytes(invalidBytes)
	assert.Error(t, err)
	assert.Equal(t, "invalid byte length for Hash: 63", err.Error())
}
