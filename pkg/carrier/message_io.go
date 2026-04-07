package carrier

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// maxMessageSize caps the on-wire message length at
// 1 MiB to prevent resource exhaustion.
const maxMessageSize = 1 << 20 // A

// Wire format for both streams and datagrams:
//
//	[1 byte: MessageType as uint8][N bytes: payload]
//
// Stream framing adds a 4-byte big-endian length
// prefix for the combined (1 + N) bytes before the
// type byte and payload:
//
//	[4 bytes: total length][1 byte: type][N bytes: payload]
//
// The type byte is read first and used to select the
// correct protobuf decoder for the payload. No proto
// envelope wraps the routing byte.

// marshalMessage encodes msg as [type][payload] for
// unreliable datagrams. The caller owns the returned
// slice.
func marshalMessage( // A
	msg interfaces.Message,
) ([]byte, error) {
	total := 1 + len(msg.Payload)
	out := make([]byte, total)
	out[0] = byte(msg.Type)
	copy(out[1:], msg.Payload)
	return out, nil
}

// unmarshalMessage decodes a [type][payload] datagram.
// The returned Payload slice aliases data[1:]; copy if
// the buffer will be reused.
func unmarshalMessage( // A
	data []byte,
) (interfaces.Message, error) {
	if len(data) < 1 {
		return interfaces.Message{}, fmt.Errorf(
			"datagram too short: 0 bytes",
		)
	}
	return interfaces.Message{
		Type:    interfaces.MessageType(data[0]),
		Payload: data[1:],
	}, nil
}

// writeMessageStream writes a framed message to
// stream using the wire format:
//
//	[4-byte big-endian total length][1-byte type][payload]
//
// It issues three Write calls in order and avoids
// allocating a single flat buffer for the frame.
func writeMessageStream( // A
	stream interfaces.Stream,
	msg interfaces.Message,
) error {
	payloadLen := len(msg.Payload)
	total := 1 + payloadLen
	if total > maxMessageSize {
		return fmt.Errorf(
			"message too large: %d bytes",
			total,
		)
	}
	u32total, err := toUint32(total)
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], u32total)
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	typeBuf := [1]byte{byte(msg.Type)}
	if _, err := stream.Write(typeBuf[:]); err != nil {
		return fmt.Errorf("write type: %w", err)
	}
	if payloadLen > 0 {
		if _, err := stream.Write(msg.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}

// readMessageStream reads a framed message from
// stream. It reads the 4-byte length prefix, validates
// bounds, reads the type byte, then reads the rest of
// the payload directly without copying into a proto
// envelope.
func readMessageStream( // A
	stream interfaces.Stream,
) (interfaces.Message, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"read length: %w", err,
		)
	}
	total := binary.BigEndian.Uint32(lenBuf[:])
	// total must be at least 1 (the type byte itself)
	// and at most maxMessageSize.
	if total == 0 || total > maxMessageSize {
		return interfaces.Message{}, fmt.Errorf(
			"message size %d out of bounds", total,
		)
	}
	var typeBuf [1]byte
	if _, err := io.ReadFull(
		stream, typeBuf[:],
	); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"read type: %w", err,
		)
	}
	msgType := interfaces.MessageType(typeBuf[0])
	payloadLen := total - 1
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(
			stream, payload,
		); err != nil {
			return interfaces.Message{}, fmt.Errorf(
				"read payload: %w", err,
			)
		}
	}
	return interfaces.Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

func toUint32(value int) (uint32, error) { // A
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, fmt.Errorf(
			"value %d out of uint32 range", value,
		)
	}
	return uint32(value), nil
}
