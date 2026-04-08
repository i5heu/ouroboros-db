package transport

import (
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
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
	out[0] = byte(msg.Type) //#nosec G115 // safe: MessageType is a small enum value
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
	typeBuf := [1]byte{byte(msg.Type)} //#nosec G115 // safe: MessageType is a small enum value
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

func writeMessageReply( // A
	stream interfaces.Stream,
	payload proto.Message,
	handlerErr error,
) error {
	reply := &pb.MessageReply{
		Success: handlerErr == nil,
	}
	if handlerErr != nil {
		reply.Error = handlerErr.Error()
	}
	if payload != nil {
		encodedPayload, err := proto.Marshal(payload)
		if err != nil {
			return fmt.Errorf(
				"marshal reply payload: %w",
				err,
			)
		}
		reply.Payload = encodedPayload
	}
	data, err := proto.Marshal(reply)
	if err != nil {
		return fmt.Errorf(
			"marshal reply envelope: %w",
			err,
		)
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf(
			"reply too large: %d bytes",
			len(data),
		)
	}
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf(
			"write reply: %w",
			err,
		)
	}
	return nil
}

func readMessageReply( // A
	stream interfaces.Stream,
) (*pb.MessageReply, error) {
	data, err := io.ReadAll(
		io.LimitReader(stream, maxMessageSize),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"read reply: %w",
			err,
		)
	}
	var reply pb.MessageReply
	if err := proto.Unmarshal(data, &reply); err != nil {
		return nil, fmt.Errorf(
			"decode reply: %w",
			err,
		)
	}
	return &reply, nil
}
