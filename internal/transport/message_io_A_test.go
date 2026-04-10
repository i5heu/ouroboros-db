package transport

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

var benchPayloadSizes = []int{
	0,
	64,
	4096,
	65536,
	524288,
	1<<20 - 1,
}

func TestMarshalMessageZeroPayload(t *testing.T) {
	msg := interfaces.Message{}
	result := marshalMessage(msg)

	if len(result) != 1 {
		t.Fatalf("expected 1 byte, got %d", len(result))
	}
	if result[0] != 0 {
		t.Errorf("expected type byte 0, got %d", result[0])
	}
}

func TestMarshalMessageOneBytePayload(t *testing.T) {
	msg := interfaces.Message{
		Type:    interfaces.MessageTypeUserMessage,
		Payload: []byte{0x42},
	}
	result := marshalMessage(msg)

	if len(result) != 2 {
		t.Errorf("expected 2 bytes, got %d", len(result))
	}
	if result[0] != byte(interfaces.MessageTypeUserMessage) {
		t.Errorf("expected type byte %d, got %d", byte(interfaces.MessageTypeUserMessage), result[0])
	}
	if result[1] != 0x42 {
		t.Errorf("expected payload byte 0x42, got 0x%x", result[1])
	}
}

func TestMarshalMessageEmpty(t *testing.T) {
	msg := interfaces.Message{}
	result := marshalMessage(msg)

	if !(len(result) == 1 && result[0] == 0) {
		t.Errorf("expected [0], got %d : %v", len(result), result)
	}
}

func TestMarshalMessageRoundTrip(t *testing.T) {
	testCases := []struct {
		name    string
		msgType interfaces.MessageType
		payload []byte
	}{
		{"zero_payload", interfaces.MessageTypeHeartbeat, []byte{}},
		{"one_byte_payload", interfaces.MessageTypeUserMessage, []byte{0x01}},
		{"small_payload", interfaces.MessageTypeBlockSliceRequest, []byte("hello")},
		{"empty_type", interfaces.MessageTypeBlockSliceResponse, []byte{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			msg := interfaces.Message{
				Type:    tc.msgType,
				Payload: tc.payload,
			}
			data := marshalMessage(msg)
			if len(data) != 1+len(tc.payload) {
				t.Errorf("marshal length mismatch: got %d, want %d", len(data), 1+len(tc.payload))
			}

			unmarshaled, err := unmarshalMessage(data)
			if err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if unmarshaled.Type != tc.msgType {
				t.Errorf("type mismatch: got %d, want %d", unmarshaled.Type, tc.msgType)
			}
			if !bytes.Equal(unmarshaled.Payload, tc.payload) {
				t.Errorf("payload mismatch: got %v, want %v", unmarshaled.Payload, tc.payload)
			}
		})
	}
}

func TestUnmarshalMessageZeroBytes(t *testing.T) {
	_, err := unmarshalMessage([]byte{})
	if err == nil {
		t.Error("expected error for empty data")
	}
	if err.Error() != "message too short: 0 bytes" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnmarshalMessageOneByte(t *testing.T) {
	data := []byte{byte(interfaces.MessageTypeHeartbeat)}
	msg, err := unmarshalMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Type != interfaces.MessageTypeHeartbeat {
		t.Errorf("expected type %d, got %d", interfaces.MessageTypeHeartbeat, msg.Type)
	}
	if len(msg.Payload) != 0 {
		t.Errorf("expected empty payload, got %v", msg.Payload)
	}
}

var (
	benchSinkMsg   interfaces.Message
	benchSinkBytes []byte
	benchSinkErr   error
)

func BenchmarkMarshalMessage(b *testing.B) { // A
	for _, size := range benchPayloadSizes {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			msg := interfaces.Message{
				Type:    interfaces.MessageTypeHeartbeat,
				Payload: bytes.Repeat([]byte{0xAA}, size),
			}
			b.SetBytes(int64(1 + size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkBytes = marshalMessage(msg)
			}
		})
	}
}

func BenchmarkUnmarshalMessage(b *testing.B) { // A
	for _, size := range benchPayloadSizes {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			wireData := append(
				[]byte{byte(interfaces.MessageTypeHeartbeat)},
				bytes.Repeat([]byte{0xAA}, size)...,
			)
			b.SetBytes(int64(len(wireData)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchSinkMsg, benchSinkErr = unmarshalMessage(wireData)
				_ = benchSinkMsg
			}
		})
	}
}

func BenchmarkWriteMessageStream(b *testing.B) { // A
	for _, size := range benchPayloadSizes {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			msg := interfaces.Message{
				Type:    interfaces.MessageTypeHeartbeat,
				Payload: bytes.Repeat([]byte{0xAA}, size),
			}
			b.SetBytes(int64(1 + size))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stream := newTestStream(nil)
				benchSinkErr = writeMessageStream(stream, msg)
				benchSinkBytes = stream.writes.Bytes()
			}
		})
	}
}

func BenchmarkWriteMessageStreamLargePayload(b *testing.B) { // A
	msg := interfaces.Message{
		Type:    interfaces.MessageTypeHeartbeat,
		Payload: bytes.Repeat([]byte{0xAA}, 1<<20-1),
	}
	stream := newTestStream(nil)
	b.SetBytes(int64(1 + len(msg.Payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream.writes.Reset()
		benchSinkErr = writeMessageStream(stream, msg)
		benchSinkBytes = stream.writes.Bytes()
	}
}

func BenchmarkReadMessageStream(b *testing.B) { // A
	for _, size := range benchPayloadSizes {
		b.Run(fmt.Sprintf("payload_%d", size), func(b *testing.B) {
			wireData := append(
				[]byte{byte(interfaces.MessageTypeHeartbeat)},
				bytes.Repeat([]byte{0xAA}, size)...,
			)
			b.SetBytes(int64(len(wireData)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				stream := newTestStream(wireData)
				benchSinkMsg, benchSinkErr = readMessageStream(stream)
				_ = benchSinkMsg
			}
		})
	}
}
