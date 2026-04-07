package carrier

import (
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// maxMessageSize caps any single message at 1 MiB to
// prevent resource exhaustion. Applied via LimitReader
// on stream reads and checked on datagram length.
const maxMessageSize = 1 << 20 // A

// Wire format — identical for streams and datagrams:
//
//	[1 byte: MessageType as uint8][N bytes: protobuf payload]
//
// Streams use one QUIC stream per message; the stream's
// natural EOF delimits the frame so no length prefix is
// needed. The type byte is read first and routes the
// remaining bytes to the correct protobuf decoder.

// marshalMessage encodes msg as [type][payload].
// Used for unreliable datagrams where the caller
// already has a flat []byte to send.
func marshalMessage( // A
	msg interfaces.Message,
) ([]byte, error) {
	out := make([]byte, 1+len(msg.Payload))
	out[0] = byte(msg.Type)
	copy(out[1:], msg.Payload)
	return out, nil
}

// unmarshalMessage decodes a [type][payload] slice.
// The returned Payload aliases data[1:]; copy it if
// the source buffer will be reused.
func unmarshalMessage( // A
	data []byte,
) (interfaces.Message, error) {
	if len(data) < 1 {
		return interfaces.Message{},
			fmt.Errorf("message too short: 0 bytes")
	}
	return interfaces.Message{
		Type:    interfaces.MessageType(data[0]),
		Payload: data[1:],
	}, nil
}

// writeMessageStream writes [type][payload] to stream
// using two Write calls and relies on the QUIC stream
// close to signal the end of the message.
func writeMessageStream( // A
	stream interfaces.Stream,
	msg interfaces.Message,
) error {
	if 1+len(msg.Payload) > maxMessageSize {
		return fmt.Errorf(
			"message too large: %d bytes",
			1+len(msg.Payload),
		)
	}
	typeBuf := [1]byte{byte(msg.Type)}
	if _, err := stream.Write(typeBuf[:]); err != nil {
		return fmt.Errorf("write type: %w", err)
	}
	if len(msg.Payload) > 0 {
		if _, err := stream.Write(msg.Payload); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
	}
	return nil
}

// readMessageStream reads [type][payload] from stream.
// It reads the type byte first, then drains the rest
// of the stream (bounded by maxMessageSize) as the
// payload. The stream's EOF is the natural frame end.
func readMessageStream( // A
	stream interfaces.Stream,
) (interfaces.Message, error) {
	var typeBuf [1]byte
	if _, err := io.ReadFull(stream, typeBuf[:]); err != nil {
		return interfaces.Message{},
			fmt.Errorf("read type: %w", err)
	}
	payload, err := io.ReadAll(
		io.LimitReader(stream, maxMessageSize),
	)
	if err != nil {
		return interfaces.Message{},
			fmt.Errorf("read payload: %w", err)
	}
	return interfaces.Message{
		Type:    interfaces.MessageType(typeBuf[0]),
		Payload: payload,
	}, nil
}
