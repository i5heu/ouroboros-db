package encoding

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeContentWithMimeType(t *testing.T) { // A
	content := []byte("sample payload")

	toLongMime := strings.Repeat("a", payloadHeaderContentSizeLen+1)

	tests := []struct {
		name     string
		mime     string
		expected bool // whether a error should be expected
		length   int
	}{
		{"mime", "text/plain", false, payloadHeaderSize + len(content)},
		{"empty", "", false, len(content) + 1},
		{"whitespace", "   ", false, len(content) + 1},
		{"tabs and newlines", "\n\t  \r", false, len(content) + 1},
		{"mixed whitespace", "  \t \n ", false, len(content) + 1},
		{"mime too long", toLongMime, true, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := EncodeContentWithMimeType(content, tc.mime)

			if tc.expected {
				// expecting an error
				if err == nil {
					t.Fatalf("expected error for mime %q but got none", tc.mime)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error for mime %q: %v", tc.mime, err)
			}

			if got := len(encoded); got != tc.length {
				t.Fatalf("unexpected encoded length for mime %q: want %d, got %d", tc.mime, tc.length, got)
			}

			// Validate header byte and payload depending on whether we expect a MIME header
			trimmed := strings.TrimSpace(tc.mime)
			if trimmed == "" {
				// no mime: should be single-byte header followed by content
				if encoded[0] != payloadHeaderNotExisting {
					t.Fatalf("expected header byte 0x%02x, got 0x%02x for mime %q", payloadHeaderNotExisting, encoded[0], tc.mime)
				}
				if !bytes.Equal(encoded[1:], content) {
					t.Fatalf("expected content to follow single-byte header for mime %q", tc.mime)
				}
				// Use DecodeContent to confirm round-trip behavior
				decoded, payloadHeader, isMime, decErr := DecodeContent(encoded)
				if decErr != nil {
					t.Fatalf("decodeContent failed: %v", decErr)
				}
				if !bytes.Equal(decoded, content) {
					t.Fatalf("decoded content mismatch for mime %q: want %q got %q", tc.mime, content, decoded)
				}
				if isMime {
					t.Fatalf("expected isMime false for no-mime case, got true")
				}
				if payloadHeader != nil {
					t.Fatalf("expected nil payloadHeader for no-mime case, got %v", payloadHeader)
				}
			} else {
				// With a MIME header we expect a full fixed-size header and trimmed mime in it
				if encoded[0] != payloadHeaderIsMime {
					t.Fatalf("expected header byte 0x%02x, got 0x%02x for mime %q", payloadHeaderIsMime, encoded[0], tc.mime)
				}
				headMime := string(bytes.TrimRight(encoded[1:payloadHeaderSize], "\x00"))
				if strings.TrimSpace(headMime) != trimmed {
					t.Fatalf("header mime mismatch for mime %q: expected %q got %q", tc.mime, trimmed, headMime)
				}
				if !bytes.Equal(encoded[payloadHeaderSize:], content) {
					t.Fatalf("expected content after payload header for mime %q", tc.mime)
				}
				// DecodeContent should return the trimmed mime in payloadHeader
				decoded, payloadHeader, isMime, decErr := DecodeContent(encoded)
				if decErr != nil {
					t.Fatalf("DecodeContent failed: %v", decErr)
				}
				if !bytes.Equal(decoded, content) {
					t.Fatalf("decoded content mismatch for mime %q: want %q got %q", tc.mime, content, decoded)
				}
				if !isMime {
					t.Fatalf("expected isMime true for mime case, got false")
				}
				mimeOut := strings.TrimSpace(string(bytes.TrimRight(payloadHeader, "\x00")))
				if mimeOut != trimmed {
					t.Fatalf("decoded mime mismatch for mime %q: want %q got %q", tc.mime, trimmed, mimeOut)
				}
			}
		})
	}
}

func TestDecodeContent(t *testing.T) { // A
	content := []byte("test payload data")

	tests := []struct {
		name         string
		payload      []byte
		expectError  bool
		expectData   []byte
		expectMime   string
		expectIsMime bool
	}{
		{
			name:        "empty_payload",
			payload:     []byte{},
			expectError: true,
		},
		{
			name:         "single_byte_no_header",
			payload:      []byte{payloadHeaderNotExisting},
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "",
			expectIsMime: false,
		},
		{
			name:         "no_header_with_content",
			payload:      append([]byte{payloadHeaderNotExisting}, content...),
			expectError:  false,
			expectData:   content,
			expectMime:   "",
			expectIsMime: false,
		},
		{
			name:        "invalid_flag_byte",
			payload:     []byte{0xFF, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "existing_flag_but_no_mime_flag",
			payload:     append([]byte{payloadHeaderExisting}, content...),
			expectError: true,
		},
		{
			name:        "mime_flag_but_payload_too_short",
			payload:     []byte{payloadHeaderIsMime, 't', 'e', 'x', 't'},
			expectError: true,
		},
		{
			name: "mime_header_exactly_at_boundary",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "",
			expectIsMime: true,
		},
		{
			name: "valid_mime_header_with_text/plain",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("text/plain"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "text/plain",
			expectIsMime: true,
		},
		{
			name: "valid_mime_header_with_application/json",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/json"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "application/json",
			expectIsMime: true,
		},
		{
			name: "mime_header_with_padding_zeros",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("image/png"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "image/png",
			expectIsMime: true,
		},
		{
			name: "mime_header_filled_to_maximum",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				maxMime := strings.Repeat("a", payloadHeaderContentSizeLen)
				copy(header[1:], []byte(maxMime))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   strings.Repeat("a", payloadHeaderContentSizeLen),
			expectIsMime: true,
		},
		{
			name: "mime_header_with_empty_content",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("text/html"))
				return header
			}(),
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "text/html",
			expectIsMime: true,
		},
		{
			name: "mime_header_with_binary_content",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/octet-stream"))
				binaryData := []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF}
				return append(header, binaryData...)
			}(),
			expectError:  false,
			expectData:   []byte{0x00, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF},
			expectMime:   "application/octet-stream",
			expectIsMime: true,
		},
		{
			name:        "random_invalid_flag_combinations",
			payload:     []byte{0x30, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "flag_0x01_invalid",
			payload:     []byte{0x01, 0x01, 0x02},
			expectError: true,
		},
		{
			name:        "flag_0x11_invalid",
			payload:     []byte{0x11, 0x01, 0x02},
			expectError: true,
		},
		{
			name: "payloadHeaderExisting_flag_with_full_header_but_should_fail",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderExisting
				copy(header[1:], []byte("text/plain"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "text/plain",
			expectIsMime: false,
		},
		{
			name: "mime_with_special_characters",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				copy(header[1:], []byte("application/vnd.api+json"))
				return append(header, content...)
			}(),
			expectError:  false,
			expectData:   content,
			expectMime:   "application/vnd.api+json",
			expectIsMime: true,
		},
		{
			name: "mime_header_exactly_payloadHeaderSize_bytes",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize)
				header[0] = payloadHeaderIsMime
				return header
			}(),
			expectError:  false,
			expectData:   []byte{},
			expectMime:   "",
			expectIsMime: true,
		},
		{
			name: "mime_header_one_byte_short",
			payload: func() []byte {
				header := make([]byte, payloadHeaderSize-1)
				header[0] = payloadHeaderIsMime
				return header
			}(),
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, payloadHeader, isMime, err := DecodeContent(tc.payload)

			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !bytes.Equal(data, tc.expectData) {
				t.Fatalf("data mismatch: want %q, got %q", tc.expectData, data)
			}

			if isMime != tc.expectIsMime {
				t.Fatalf("isMime mismatch: want %v, got %v", tc.expectIsMime, isMime)
			}

			if tc.expectIsMime || tc.payload[0] == payloadHeaderExisting {
				actualMime := string(bytes.TrimRight(payloadHeader, "\x00"))
				if actualMime != tc.expectMime {
					t.Fatalf("mime type mismatch: want %q, got %q", tc.expectMime, actualMime)
				}
			} else {
				if payloadHeader != nil {
					t.Fatalf("expected nil payloadHeader for non-mime case, got %v", payloadHeader)
				}
			}
		})
	}
}
