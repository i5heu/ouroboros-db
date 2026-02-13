// Package auth provides authentication and authorization
// for OuroborosDB, including SHA-256 based hash types for CA
// identifiers.
package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// CaHash is a fixed-size array representing a SHA-256 hash
// used for Certificate Authority (CA) identifiers.
type CaHash [sha256.Size]byte

// HashBytes computes the SHA-256 hash of the given data
// and returns it as a CaHash.
func HashBytes(data []byte) CaHash { // A
	return CaHash(sha256.Sum256(data))
}

// HashHexadecimal parses a hexadecimal string and returns
// the corresponding CaHash. Returns an error if the string
// is not a valid 64-character hex representation.
func HashHexadecimal(s string) (CaHash, error) { // A
	if len(s) != sha256.Size*2 {
		return CaHash{}, fmt.Errorf(
			"invalid hex length: expected %d, got %d",
			sha256.Size*2, len(s),
		)
	}

	decoded, err := hex.DecodeString(s)
	if err != nil {
		return CaHash{}, fmt.Errorf(
			"decode hex: %w", err,
		)
	}

	var h CaHash
	copy(h[:], decoded)
	return h, nil
}

// HashString computes the SHA-256 hash of the given string.
func HashString(s string) CaHash { // A
	return HashBytes([]byte(s))
}

// Equal returns true if this hash equals the other hash.
func (h CaHash) Equal(other CaHash) bool { // A
	return subtle.ConstantTimeCompare(
		h[:],
		other[:],
	) == 1
}

// IsZero returns true if this hash is the zero value
// (all bytes are zero).
func (h CaHash) IsZero() bool { // A
	return h == CaHash{}
}

// Bytes returns a byte slice copy of the hash.
func (h CaHash) Bytes() []byte { // A
	b := make([]byte, len(h))
	copy(b, h[:])
	return b
}

// String returns the hexadecimal string representation
// of the hash.
func (h CaHash) String() string { // A
	return hex.EncodeToString(h[:])
}

// Hex returns the hexadecimal string representation
// of the hash (alias for String).
func (h CaHash) Hex() string { // A
	return h.String()
}
