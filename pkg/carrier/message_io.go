package carrier

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const maxMessageSize = 1 << 20 // A

func marshalMessage( // A
	msg interfaces.Message,
) ([]byte, error) {
	return cbor.Marshal(msg)
}

func unmarshalMessage( // A
	data []byte,
) (interfaces.Message, error) {
	var msg interfaces.Message
	if err := cbor.Unmarshal(data, &msg); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"decode message: %w",
			err,
		)
	}
	return msg, nil
}

func writeMessageStream( // A
	stream interfaces.Stream,
	msg interfaces.Message,
) error {
	data, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf(
			"message too large: %d bytes",
			len(data),
		)
	}
	payloadLen, err := toUint32(len(data))
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], payloadLen)
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

func readMessageStream( // A
	stream interfaces.Stream,
) (interfaces.Message, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"read length: %w",
			err,
		)
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen == 0 || msgLen > maxMessageSize {
		return interfaces.Message{}, fmt.Errorf(
			"message size %d out of bounds",
			msgLen,
		)
	}
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, buf); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"read payload: %w",
			err,
		)
	}
	return unmarshalMessage(buf)
}

func toUint32(value int) (uint32, error) { // A
	if value < 0 || uint64(value) > math.MaxUint32 {
		return 0, fmt.Errorf(
			"value %d out of uint32 range",
			value,
		)
	}
	return uint32(value), nil
}
