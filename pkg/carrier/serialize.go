package carrier

import (
	"bytes"
	"encoding/gob"
	"fmt"
)

// Serialize encodes any payload type to bytes using GOB encoding.
// This provides a unified serialization approach for all payload types.
func Serialize[T any](v T) ([]byte, error) { // A
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(v); err != nil {
		return nil, fmt.Errorf("serialize %T: %w", v, err)
	}
	return buf.Bytes(), nil
}

// Deserialize decodes bytes to the specified payload type using GOB encoding.
// The type parameter T must match the type that was originally serialized.
func Deserialize[T any](data []byte) (T, error) { // A
	var v T
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&v); err != nil {
		return v, fmt.Errorf("deserialize %T: %w", v, err)
	}
	return v, nil
}

// MustSerialize is like Serialize but panics on error.
// Use only when serialization failure is a programming error.
func MustSerialize[T any](v T) []byte { // A
	data, err := Serialize(v)
	if err != nil {
		panic(err)
	}
	return data
}
