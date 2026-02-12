package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"pgregory.net/rapid"
)

func TestHashBytes(t *testing.T) { // A
	data := []byte("test data")
	h := HashBytes(data)

	expected := sha256.Sum256(data)
	if h != CaHash(expected) {
		t.Error("HashBytes produced unexpected result")
	}
}

func TestHashBytesEmpty(t *testing.T) { // A
	h1 := HashBytes([]byte{})
	h2 := HashBytes(nil)

	if h1 != h2 {
		t.Error("empty and nil should produce same hash")
	}

	expected := sha256.Sum256([]byte{})
	if h1 != CaHash(expected) {
		t.Error("empty data hash mismatch")
	}
}

func TestHashBytesDeterministic(t *testing.T) { // A
	data := []byte("deterministic test")
	h1 := HashBytes(data)
	h2 := HashBytes(data)

	if h1 != h2 {
		t.Error("HashBytes not deterministic")
	}
}

func TestHashBytesDifferentData(t *testing.T) { // A
	h1 := HashBytes([]byte("data1"))
	h2 := HashBytes([]byte("data2"))

	if h1 == h2 {
		t.Error("different data should produce different hashes")
	}
}

func TestHashString(t *testing.T) { // A
	s := "test string"
	h := HashString(s)

	expected := HashBytes([]byte(s))
	if h != expected {
		t.Error("HashString mismatch with HashBytes")
	}
}

func TestHashHexadecimalValid(t *testing.T) { // A
	data := []byte("test")
	original := HashBytes(data)
	hexStr := original.Hex()

	parsed, err := HashHexadecimal(hexStr)
	if err != nil {
		t.Fatalf("HashHexadecimal: %v", err)
	}

	if parsed != original {
		t.Error("parsed hash does not match original")
	}
}

func TestHashHexadecimalInvalidLength(t *testing.T) { // A
	_, err := HashHexadecimal("abc123") // too short
	if err == nil {
		t.Error("expected error for invalid length")
	}
}

func TestHashHexadecimalInvalidChars(t *testing.T) { // A
	_, err := HashHexadecimal(
		"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz" +
			"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz",
	)
	if err == nil {
		t.Error("expected error for invalid hex chars")
	}
}

func TestCaHashEqual(t *testing.T) { // A
	h1 := HashBytes([]byte("test"))
	h2 := HashBytes([]byte("test"))
	h3 := HashBytes([]byte("different"))

	if !h1.Equal(h2) {
		t.Error("equal hashes should be equal")
	}

	if h1.Equal(h3) {
		t.Error("different hashes should not be equal")
	}
}

func TestCaHashIsZero(t *testing.T) { // A
	zero := CaHash{}
	nonZero := HashBytes([]byte("test"))

	if !zero.IsZero() {
		t.Error("zero hash should be zero")
	}

	if nonZero.IsZero() {
		t.Error("non-zero hash should not be zero")
	}
}

func TestCaHashBytes(t *testing.T) { // A
	h := HashBytes([]byte("test"))
	b := h.Bytes()

	if len(b) != sha256.Size {
		t.Errorf("expected %d bytes, got %d", sha256.Size, len(b))
	}

	for i := 0; i < sha256.Size; i++ {
		if b[i] != h[i] {
			t.Errorf("byte mismatch at index %d", i)
		}
	}
}

func TestCaHashBytesCopy(t *testing.T) { // A
	h := HashBytes([]byte("test"))
	b := h.Bytes()

	// Modify the slice
	b[0] = 0xFF

	// Original should be unchanged
	if h[0] == 0xFF {
		t.Error("Bytes() should return a copy, not reference")
	}
}

func TestCaHashString(t *testing.T) { // A
	h := HashBytes([]byte("test"))
	s := h.String()

	if len(s) != sha256.Size*2 {
		t.Errorf("expected hex string length %d, got %d", sha256.Size*2, len(s))
	}

	// Verify it's valid hex
	decoded, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("String() produced invalid hex: %v", err)
	}

	if len(decoded) != sha256.Size {
		t.Errorf("decoded length mismatch: expected %d, got %d", sha256.Size, len(decoded))
	}
}

func TestCaHashHex(t *testing.T) { // A
	h := HashBytes([]byte("test"))

	if h.Hex() != h.String() {
		t.Error("Hex() should equal String()")
	}
}

func TestCaHashSize(t *testing.T) { // A
	var h CaHash
	if len(h) != sha256.Size {
		t.Errorf("CaHash size should be %d, got %d", sha256.Size, len(h))
	}
}

// Property-based tests

func TestHashBytesPropertyDeterministic(t *testing.T) { // A
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOf(rapid.Byte()).Draw(t, "data")

		h1 := HashBytes(data)
		h2 := HashBytes(data)

		if h1 != h2 {
			t.Error("HashBytes not deterministic")
		}
	})
}

func TestHashBytesPropertyCollision(t *testing.T) { // A
	rapid.Check(t, func(t *rapid.T) {
		data1 := rapid.SliceOf(rapid.Byte()).Draw(t, "data1")
		data2 := rapid.SliceOf(rapid.Byte()).Draw(t, "data2")

		// Skip if data is equal
		if string(data1) == string(data2) {
			return
		}

		h1 := HashBytes(data1)
		h2 := HashBytes(data2)

		if h1 == h2 {
			t.Error("hash collision detected")
		}
	})
}

func TestHashHexadecimalRoundTrip(t *testing.T) { // A
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOf(rapid.Byte()).Draw(t, "data")

		original := HashBytes(data)
		hexStr := original.Hex()

		parsed, err := HashHexadecimal(hexStr)
		if err != nil {
			t.Fatalf("parse hex: %v", err)
		}

		if parsed != original {
			t.Error("round-trip failed")
		}
	})
}

func TestCaHashPropertyEqual(t *testing.T) { // A
	rapid.Check(t, func(t *rapid.T) {
		data := rapid.SliceOf(rapid.Byte()).Draw(t, "data")

		h1 := HashBytes(data)
		h2 := HashBytes(data)

		if !h1.Equal(h2) {
			t.Error("hashes of same data should be equal")
		}
	})
}
