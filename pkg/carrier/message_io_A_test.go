package carrier

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

var benchSinkMsg interfaces.Message
var benchSinkBytes []byte
var benchSinkErr error

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
				benchSinkBytes, benchSinkErr = marshalMessage(msg)
				_ = benchSinkErr
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
