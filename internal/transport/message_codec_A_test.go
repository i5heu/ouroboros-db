package transport

import (
	"bytes"
	"errors"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"pgregory.net/rapid"
)

func TestWriteReadRoundTrip(t *testing.T) { // A
	t.Parallel()
	tests := []struct {
		name string
		msg  interfaces.Message
	}{
		{
			name: "empty payload",
			msg: interfaces.Message{
				Type:    interfaces.MessageTypeHeartbeat,
				Payload: nil,
			},
		},
		{
			name: "small payload",
			msg: interfaces.Message{
				Type:    interfaces.MessageTypeBlockSliceRequest,
				Payload: []byte("hello"),
			},
		},
		{
			name: "binary payload",
			msg: interfaces.Message{
				Type: interfaces.MessageTypeLogPush,
				Payload: []byte{
					0x00, 0xff, 0x01, 0xfe,
				},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteMessage(&buf, tc.msg)
			if err != nil {
				t.Fatalf(
					"WriteMessage: %v", err,
				)
			}
			got, err := ReadMessage(&buf)
			if err != nil {
				t.Fatalf(
					"ReadMessage: %v", err,
				)
			}
			if got.Type != tc.msg.Type {
				t.Errorf(
					"type: got %d, want %d",
					got.Type,
					tc.msg.Type,
				)
			}
			if !bytes.Equal(
				got.Payload,
				tc.msg.Payload,
			) {
				t.Errorf(
					"payload mismatch",
				)
			}
		})
	}
}

func TestSerializeDeserializeRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	tests := []struct {
		name string
		msg  interfaces.Message
	}{
		{
			name: "empty payload",
			msg: interfaces.Message{
				Type: interfaces.MessageTypeHeartbeat,
			},
		},
		{
			name: "with payload",
			msg: interfaces.Message{
				Type:    interfaces.MessageTypeLogPush,
				Payload: []byte("test data"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := SerializeMessage(tc.msg)
			if err != nil {
				t.Fatalf(
					"SerializeMessage: %v", err,
				)
			}
			got, err := DeserializeMessage(data)
			if err != nil {
				t.Fatalf(
					"DeserializeMessage: %v", err,
				)
			}
			if got.Type != tc.msg.Type {
				t.Errorf(
					"type: got %d, want %d",
					got.Type,
					tc.msg.Type,
				)
			}
			if !bytes.Equal(
				got.Payload,
				tc.msg.Payload,
			) {
				t.Errorf("payload mismatch")
			}
		})
	}
}

func TestDeserializeTooShort(t *testing.T) { // A
	t.Parallel()
	_, err := DeserializeMessage([]byte{0x01})
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

func TestDeserializeLengthMismatch( // A
	t *testing.T,
) {
	t.Parallel()
	data := []byte{
		0, 0, 0, 0,
		0, 0, 0, 10,
		1, 2, 3,
	}
	_, err := DeserializeMessage(data)
	if err == nil {
		t.Fatal("expected error for length mismatch")
	}
}

func TestPropertyRoundTrip(t *testing.T) { // A
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		msgType := interfaces.MessageType(
			rapid.IntRange(0, 13).Draw(t, "type"),
		)
		payloadLen := rapid.IntRange(
			0, 4096,
		).Draw(t, "len")
		payload := rapid.SliceOfN(
			rapid.Byte(),
			payloadLen,
			payloadLen,
		).Draw(t, "payload")

		msg := interfaces.Message{
			Type:    msgType,
			Payload: payload,
		}

		// stream round-trip
		var buf bytes.Buffer
		if err := WriteMessage(&buf, msg); err != nil {
			t.Fatalf("WriteMessage: %v", err)
		}
		got, err := ReadMessage(&buf)
		if err != nil {
			t.Fatalf("ReadMessage: %v", err)
		}
		if got.Type != msg.Type {
			t.Fatalf(
				"stream type: %d != %d",
				got.Type,
				msg.Type,
			)
		}
		if !bytes.Equal(got.Payload, msg.Payload) {
			t.Fatal("stream payload mismatch")
		}

		// datagram round-trip
		data, err := SerializeMessage(msg)
		if err != nil {
			t.Fatalf("SerializeMessage: %v", err)
		}
		got2, err := DeserializeMessage(data)
		if err != nil {
			t.Fatalf("DeserializeMessage: %v", err)
		}
		if got2.Type != msg.Type {
			t.Fatalf(
				"dgram type: %d != %d",
				got2.Type,
				msg.Type,
			)
		}
		if !bytes.Equal(got2.Payload, msg.Payload) {
			t.Fatal("dgram payload mismatch")
		}
	})
}

// respTestCase holds expectations for a response
// round-trip assertion.
type respTestCase struct { // A
	name     string
	resp     interfaces.Response
	wantErr  bool
	wantMeta map[string]string
}

// assertRespRoundTrip writes and reads a response,
// then checks payload, error, and metadata.
func assertRespRoundTrip( // A
	t *testing.T,
	tc respTestCase,
) {
	t.Helper()
	var buf bytes.Buffer
	err := WriteResponse(&buf, tc.resp)
	if err != nil {
		t.Fatalf("WriteResponse: %v", err)
	}
	got, err := ReadResponse(&buf)
	if err != nil {
		t.Fatalf("ReadResponse: %v", err)
	}
	assertRespFields(t, got, tc)
}

// assertRespFields compares got against expected
// fields from the test case.
func assertRespFields( // A
	t *testing.T,
	got interfaces.Response,
	tc respTestCase,
) {
	t.Helper()
	if !bytes.Equal(got.Payload, tc.resp.Payload) {
		t.Errorf(
			"payload: got %q, want %q",
			got.Payload, tc.resp.Payload,
		)
	}
	assertRespError(t, got, tc)
	assertRespMeta(t, got, tc)
}

// assertRespError checks the error field.
func assertRespError( // A
	t *testing.T,
	got interfaces.Response,
	tc respTestCase,
) {
	t.Helper()
	if tc.wantErr {
		if got.Error == nil {
			t.Fatal("expected error")
		}
		if got.Error.Error() !=
			tc.resp.Error.Error() {
			t.Errorf(
				"error: got %q, want %q",
				got.Error.Error(),
				tc.resp.Error.Error(),
			)
		}
	} else if got.Error != nil {
		t.Fatalf("unexpected error: %v", got.Error)
	}
}

// assertRespMeta checks the metadata map.
func assertRespMeta( // A
	t *testing.T,
	got interfaces.Response,
	tc respTestCase,
) {
	t.Helper()
	if tc.wantMeta == nil {
		return
	}
	for k, v := range tc.wantMeta {
		if got.Metadata[k] != v {
			t.Errorf(
				"metadata[%s]: got %q, want %q",
				k, got.Metadata[k], v,
			)
		}
	}
}

// TestResponseWriteReadRoundTrip verifies that
// WriteResponse/ReadResponse produce identical
// responses for various scenarios.
func TestResponseWriteReadRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	tests := []respTestCase{
		{
			name: "empty response",
			resp: interfaces.Response{},
		},
		{
			name: "payload only",
			resp: interfaces.Response{
				Payload: []byte("result"),
			},
		},
		{
			name: "error only",
			resp: interfaces.Response{
				Error: errors.New("bad request"),
			},
			wantErr: true,
		},
		{
			name: "payload and error",
			resp: interfaces.Response{
				Payload: []byte("partial"),
				Error:   errors.New("incomplete"),
			},
			wantErr: true,
		},
		{
			name: "with metadata",
			resp: interfaces.Response{
				Payload: []byte("data"),
				Metadata: map[string]string{
					"key1": "val1",
					"key2": "val2",
				},
			},
			wantMeta: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
		},
		{
			name: "full response",
			resp: interfaces.Response{
				Payload: []byte("full"),
				Error:   errors.New("warn"),
				Metadata: map[string]string{
					"code": "200",
				},
			},
			wantErr: true,
			wantMeta: map[string]string{
				"code": "200",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertRespRoundTrip(t, tc)
		})
	}
}

// TestReadResponseTooShort verifies error on
// truncated response data.
func TestReadResponseTooShort(t *testing.T) { // A
	t.Parallel()
	r := bytes.NewReader([]byte{0x01})
	_, err := ReadResponse(r)
	if err == nil {
		t.Fatal("expected error for short data")
	}
}

// drawTestResponse generates a random Response
// using rapid generators.
func drawTestResponse( // A
	t *rapid.T,
) interfaces.Response {
	payloadLen := rapid.IntRange(
		0, 4096,
	).Draw(t, "payloadLen")
	payload := rapid.SliceOfN(
		rapid.Byte(),
		payloadLen,
		payloadLen,
	).Draw(t, "payload")

	hasErr := rapid.Bool().Draw(t, "hasErr")
	var respErr error
	if hasErr {
		errMsg := rapid.StringMatching(
			`[a-z ]{1,100}`,
		).Draw(t, "errMsg")
		respErr = errors.New(errMsg)
	}

	meta := drawTestMetadata(t)

	return interfaces.Response{
		Payload:  payload,
		Error:    respErr,
		Metadata: meta,
	}
}

// drawTestMetadata generates a random metadata map
// using rapid generators.
func drawTestMetadata( // A
	t *rapid.T,
) map[string]string {
	metaCount := rapid.IntRange(
		0, 5,
	).Draw(t, "metaCount")
	if metaCount == 0 {
		return nil
	}
	meta := make(map[string]string, metaCount)
	for i := 0; i < metaCount; i++ {
		k := rapid.StringMatching(
			`[a-z]{1,20}`,
		).Draw(t, "metaKey")
		v := rapid.StringMatching(
			`[a-z0-9]{1,50}`,
		).Draw(t, "metaVal")
		meta[k] = v
	}
	return meta
}

// assertPropertyResp checks that a round-tripped
// response matches the original.
func assertPropertyResp( // A
	t *rapid.T,
	got interfaces.Response,
	want interfaces.Response,
) {
	if !bytes.Equal(got.Payload, want.Payload) {
		t.Fatal("payload mismatch")
	}
	if want.Error != nil {
		if got.Error == nil {
			t.Fatal("expected error")
		}
		if got.Error.Error() !=
			want.Error.Error() {
			t.Fatal("error message mismatch")
		}
	} else if got.Error != nil {
		t.Fatal("unexpected error")
	}
	if len(got.Metadata) != len(want.Metadata) {
		t.Fatalf(
			"metadata count: got %d, want %d",
			len(got.Metadata),
			len(want.Metadata),
		)
	}
	for k, v := range want.Metadata {
		if got.Metadata[k] != v {
			t.Fatalf("metadata[%s] mismatch", k)
		}
	}
}

// TestPropertyResponseRoundTrip uses rapid to
// verify response codec consistency.
func TestPropertyResponseRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		resp := drawTestResponse(t)

		var buf bytes.Buffer
		if err := WriteResponse(
			&buf, resp,
		); err != nil {
			t.Fatalf("WriteResponse: %v", err)
		}
		got, err := ReadResponse(&buf)
		if err != nil {
			t.Fatalf("ReadResponse: %v", err)
		}

		assertPropertyResp(t, got, resp)
	})
}
