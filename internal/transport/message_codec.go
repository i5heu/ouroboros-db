package transport

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

const (
	headerSize   = 8
	maxPayloadMB = 64
	maxPayload   = maxPayloadMB * 1024 * 1024
)

const maxUint32 = ^uint32(0)

func messageTypeToUint32( // A
	msgType interfaces.MessageType,
) (uint32, error) {
	v := int64(msgType)
	if v < 0 || v > int64(maxUint32) {
		return 0, fmt.Errorf(
			"message type out of range: %d",
			msgType,
		)
	}
	return uint32(v), nil
}

func intLenToUint32( // A
	value int,
) (uint32, error) {
	if value < 0 || uint64(value) > uint64(maxUint32) {
		return 0, fmt.Errorf(
			"length out of uint32 range: %d",
			value,
		)
	}
	// #nosec G115 -- bounds are validated just above.
	return uint32(value), nil
}

// WriteMessage serializes a Message to a writer
// using length-prefixed framing. Wire format:
// [4B type big-endian uint32]
// [4B payload length big-endian uint32]
// [N bytes payload]
func WriteMessage( // A
	w io.Writer,
	msg interfaces.Message,
) error {
	if len(msg.Payload) > maxPayload {
		return fmt.Errorf(
			"payload exceeds %dMB limit",
			maxPayloadMB,
		)
	}
	msgType, err := messageTypeToUint32(msg.Type)
	if err != nil {
		return err
	}
	payloadLen, err := intLenToUint32(len(msg.Payload))
	if err != nil {
		return err
	}
	var hdr [headerSize]byte
	binary.BigEndian.PutUint32(
		hdr[:4],
		msgType,
	)
	binary.BigEndian.PutUint32(
		hdr[4:],
		payloadLen,
	)
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if len(msg.Payload) > 0 {
		if _, err := w.Write(msg.Payload); err != nil {
			return fmt.Errorf(
				"write payload: %w",
				err,
			)
		}
	}
	return nil
}

// ReadMessage deserializes a Message from a reader.
func ReadMessage( // A
	r io.Reader,
) (interfaces.Message, error) {
	var hdr [headerSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return interfaces.Message{}, fmt.Errorf(
			"read header: %w",
			err,
		)
	}
	msgType := interfaces.MessageType(
		binary.BigEndian.Uint32(hdr[:4]),
	)
	payloadLen := binary.BigEndian.Uint32(hdr[4:])
	if payloadLen > maxPayload {
		return interfaces.Message{}, fmt.Errorf(
			"payload length %d exceeds %dMB limit",
			payloadLen,
			maxPayloadMB,
		)
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(
			r, payload,
		); err != nil {
			return interfaces.Message{}, fmt.Errorf(
				"read payload: %w",
				err,
			)
		}
	}
	return interfaces.Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// SerializeMessage converts a Message to bytes for
// datagram delivery.
func SerializeMessage( // A
	msg interfaces.Message,
) ([]byte, error) {
	if len(msg.Payload) > maxPayload {
		return nil, fmt.Errorf(
			"payload exceeds %dMB limit",
			maxPayloadMB,
		)
	}
	msgType, err := messageTypeToUint32(msg.Type)
	if err != nil {
		return nil, err
	}
	payloadLen, err := intLenToUint32(len(msg.Payload))
	if err != nil {
		return nil, err
	}
	buf := make([]byte, headerSize+len(msg.Payload))
	binary.BigEndian.PutUint32(
		buf[:4],
		msgType,
	)
	binary.BigEndian.PutUint32(
		buf[4:8],
		payloadLen,
	)
	copy(buf[headerSize:], msg.Payload)
	return buf, nil
}

// DeserializeMessage converts bytes back to a
// Message.
func DeserializeMessage( // A
	data []byte,
) (interfaces.Message, error) {
	if len(data) < headerSize {
		return interfaces.Message{}, errors.New(
			"data too short for header",
		)
	}
	msgType := interfaces.MessageType(
		binary.BigEndian.Uint32(data[:4]),
	)
	payloadLen := binary.BigEndian.Uint32(data[4:8])
	if int(payloadLen) != len(data)-headerSize {
		return interfaces.Message{}, fmt.Errorf(
			"payload length %d does not match data "+
				"length %d",
			payloadLen,
			len(data)-headerSize,
		)
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		copy(payload, data[headerSize:])
	}
	return interfaces.Message{
		Type:    msgType,
		Payload: payload,
	}, nil
}

// responseHeaderSize is the fixed overhead for a
// serialized response: 1B error flag + 4B payload
// length.
const responseHeaderSize = 5

// WriteResponse serializes a Response to a writer.
// Wire format:
//
//	[1B error flag: 0=ok, 1=error]
//	[4B payload length big-endian]
//	[N bytes payload]
//	[4B error message length] (only if flag=1)
//	[M bytes error message]   (only if flag=1)
//	[4B metadata entry count]
//	For each entry:
//	  [4B key length][key bytes]
//	  [4B value length][value bytes]
func WriteResponse( // A
	w io.Writer,
	resp interfaces.Response,
) error {
	return writeResponseData(w, resp)
}

// writeResponseData handles the actual serialization
// of a Response, split out to keep cyclomatic
// complexity manageable.
func writeResponseData( // A
	w io.Writer,
	resp interfaces.Response,
) error {
	if len(resp.Payload) > maxPayload {
		return fmt.Errorf(
			"response payload exceeds %dMB limit",
			maxPayloadMB,
		)
	}

	var hdr [responseHeaderSize]byte
	if resp.Error != nil {
		hdr[0] = 1
	}
	payloadLen, err := intLenToUint32(len(resp.Payload))
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint32(
		hdr[1:5],
		payloadLen,
	)
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf(
			"write response header: %w", err,
		)
	}

	if err := writeBytes(
		w, resp.Payload,
	); err != nil {
		return err
	}

	if err := writeResponseError(
		w, resp.Error,
	); err != nil {
		return err
	}

	return writeResponseMetadata(w, resp.Metadata)
}

// writeBytes writes a byte slice to w if non-empty.
func writeBytes( // A
	w io.Writer,
	data []byte,
) error {
	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return fmt.Errorf(
				"write bytes: %w", err,
			)
		}
	}
	return nil
}

// writeResponseError writes the error portion of a
// Response.
func writeResponseError( // A
	w io.Writer,
	respErr error,
) error {
	if respErr == nil {
		return nil
	}
	errMsg := []byte(respErr.Error())
	errLen, err := intLenToUint32(len(errMsg))
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(
		lenBuf[:], errLen,
	)
	if _, err := w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf(
			"write error length: %w", err,
		)
	}
	return writeBytes(w, errMsg)
}

// writeResponseMetadata writes the metadata map.
// Keys are sorted for deterministic output.
func writeResponseMetadata( // A
	w io.Writer,
	md map[string]string,
) error {
	entryCount, err := intLenToUint32(len(md))
	if err != nil {
		return err
	}
	var countBuf [4]byte
	binary.BigEndian.PutUint32(
		countBuf[:], entryCount,
	)
	if _, err = w.Write(countBuf[:]); err != nil {
		return fmt.Errorf(
			"write metadata count: %w", err,
		)
	}

	keys := make([]string, 0, len(md))
	for k := range md {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := writeLenPrefixed(
			w, []byte(k),
		); err != nil {
			return err
		}
		if err := writeLenPrefixed(
			w, []byte(md[k]),
		); err != nil {
			return err
		}
	}
	return nil
}

// writeLenPrefixed writes a 4-byte length prefix
// followed by the data.
func writeLenPrefixed( // A
	w io.Writer,
	data []byte,
) error {
	dataLen, err := intLenToUint32(len(data))
	if err != nil {
		return err
	}
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(
		lenBuf[:], dataLen,
	)
	if _, err = w.Write(lenBuf[:]); err != nil {
		return fmt.Errorf(
			"write length prefix: %w", err,
		)
	}
	return writeBytes(w, data)
}

// ReadResponse deserializes a Response from a
// reader.
func ReadResponse( // A
	r io.Reader,
) (interfaces.Response, error) {
	var hdr [responseHeaderSize]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return interfaces.Response{}, fmt.Errorf(
			"read response header: %w", err,
		)
	}

	hasErr := hdr[0] == 1
	payloadLen := binary.BigEndian.Uint32(hdr[1:5])
	if payloadLen > maxPayload {
		return interfaces.Response{}, fmt.Errorf(
			"response payload %d exceeds %dMB limit",
			payloadLen,
			maxPayloadMB,
		)
	}

	payload, err := readN(r, int(payloadLen))
	if err != nil {
		return interfaces.Response{}, err
	}

	var respErr error
	if hasErr {
		respErr, err = readResponseError(r)
		if err != nil {
			return interfaces.Response{}, err
		}
	}

	md, err := readResponseMetadata(r)
	if err != nil {
		return interfaces.Response{}, err
	}

	return interfaces.Response{
		Payload:  payload,
		Error:    respErr,
		Metadata: md,
	}, nil
}

// readN reads exactly n bytes from r.
func readN( // A
	r io.Reader,
	n int,
) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf(
			"read bytes: %w", err,
		)
	}
	return buf, nil
}

// readResponseError reads the error message portion.
func readResponseError( // A
	r io.Reader,
) (error, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(
		r, lenBuf[:],
	); err != nil {
		return nil, fmt.Errorf(
			"read error length: %w", err,
		)
	}
	errLen := binary.BigEndian.Uint32(lenBuf[:])
	errMsg, err := readN(r, int(errLen))
	if err != nil {
		return nil, err
	}
	return errors.New(string(errMsg)), nil
}

// readResponseMetadata reads the metadata map.
func readResponseMetadata( // A
	r io.Reader,
) (map[string]string, error) {
	var countBuf [4]byte
	if _, err := io.ReadFull(
		r, countBuf[:],
	); err != nil {
		return nil, fmt.Errorf(
			"read metadata count: %w", err,
		)
	}
	count := binary.BigEndian.Uint32(countBuf[:])
	if count == 0 {
		return nil, nil
	}

	md := make(map[string]string, count)
	for i := uint32(0); i < count; i++ {
		k, err := readLenPrefixed(r)
		if err != nil {
			return nil, err
		}
		v, err := readLenPrefixed(r)
		if err != nil {
			return nil, err
		}
		md[string(k)] = string(v)
	}
	return md, nil
}

// readLenPrefixed reads a 4-byte length prefix
// followed by that many bytes of data.
func readLenPrefixed( // A
	r io.Reader,
) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(
		r, lenBuf[:],
	); err != nil {
		return nil, fmt.Errorf(
			"read length prefix: %w", err,
		)
	}
	dataLen := binary.BigEndian.Uint32(lenBuf[:])
	return readN(r, int(dataLen))
}
