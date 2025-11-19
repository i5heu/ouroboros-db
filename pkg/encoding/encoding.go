package encoding

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

const (
	payloadHeaderSize           = 256
	payloadHeaderNotExisting    = 0x00
	payloadHeaderExisting       = 0x10
	payloadHeaderIsMime         = 0x20
	payloadHeaderContentSizeLen = payloadHeaderSize - 1
)

// encodeContentWithMimeType encodes raw content with an optional MIME type header.
//
// If mimeType is empty or contains only whitespace, the function returns a new byte slice
// that starts with a single header byte set to payloadHeaderNotExisting followed by the content.
//
// If mimeType is non-empty, it is trimmed of surrounding whitespace and validated against
// payloadHeaderContentSizeLen; if it exceeds that length the function returns an error.
// Otherwise the function builds a header of size payloadHeaderSize with the first byte
// set to payloadHeaderIsMime and the MIME type bytes copied into header[1:]. The returned
// payload is the header concatenated with the content bytes.
//
// The function does not modify its input slices and returns either the encoded payload or an error.
func EncodeContentWithMimeType(content []byte, mimeType string) ([]byte, error) { // PHC

	// check if mimeType is empty before TrimSpace to avoid unnecessary processing
	if mimeType == "" {
		encoded := append(make([]byte, 1), content...)
		encoded[0] = payloadHeaderNotExisting
		return encoded, nil
	}

	cleanMimeType := strings.TrimSpace(mimeType)
	if cleanMimeType == "" {
		encoded := append(make([]byte, 1), content...)
		encoded[0] = payloadHeaderNotExisting
		return encoded, nil
	}

	mimeBytes := []byte(cleanMimeType)
	if len(mimeBytes) > payloadHeaderContentSizeLen {
		return nil, fmt.Errorf("MIME type too long: %d bytes (max %d)", len(mimeBytes), payloadHeaderContentSizeLen)
	}
	header := make([]byte, payloadHeaderSize)

	header[0] = payloadHeaderIsMime
	copy(header[1:], mimeBytes)

	return append(header, content...), nil
}

// decodeContent parses the supplied payload into the payload data, optional header and a MIME flag.
//
// The first byte of the payload is a flag byte indicating the presence of a payload header and whether it contains a MIME type.
// If the flag indicates no payload header, the function returns the content starting from payload[1:].
// If the flag indicates a payload header with MIME type, the function extracts the header from payload[1:payloadHeaderSize]
// and returns the remaining bytes as content. If the flag combination is invalid or the payload is too short,
// an error is returned.
//
// The function does not modify its input slice and returns either the decoded content, header, MIME flag, or an error.
func DecodeContent(payload []byte) (data []byte, payloadHeader []byte, isMime bool, err error) { // PHC
	if len(payload) < 1 {
		return nil, nil, false, errors.New("ouroboros: payload is impossible short, it must be at least 1 byte")
	}

	if payload[0] == payloadHeaderNotExisting {
		return payload[1:], nil, false, nil
	}

	// If not valid flag for existing data with MIME type, return error
	if payload[0] != payloadHeaderExisting && payload[0] != payloadHeaderIsMime {
		return []byte{}, nil, false, errors.New("ouroboros: invalid payload header flag combination")
	}
	if len(payload) < payloadHeaderSize {
		return nil, nil, false, errors.New("ouroboros: payloadHeader indicated but payload too short")
	}

	payloadHeader = bytes.TrimRight(payload[1:payloadHeaderSize], "\x00")
	data = payload[payloadHeaderSize:]
	isMime = payload[0] == payloadHeaderIsMime

	return data, payloadHeader, isMime, nil
}
