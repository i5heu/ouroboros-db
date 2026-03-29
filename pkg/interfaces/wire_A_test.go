package interfaces

import "testing"

func TestWireMessageMarshalRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()

	want := NewWireMessage(
		MessageTypeHeartbeat,
		[]byte("payload"),
	)
	encoded, err := MarshalMessage(want)
	if err != nil {
		t.Fatalf("MarshalMessage: %v", err)
	}

	got, err := UnmarshalMessage(encoded)
	if err != nil {
		t.Fatalf("UnmarshalMessage: %v", err)
	}
	if got.GetType() != int32(MessageTypeHeartbeat) {
		t.Fatalf("type: got %d", got.GetType())
	}
	if string(got.GetPayload()) != "payload" {
		t.Fatalf("payload: got %q", got.GetPayload())
	}
}

func TestWireResponseMarshalRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()

	want := NewWireResponse(
		[]byte("payload"),
		"failed",
		map[string]string{"k": "v"},
	)
	encoded, err := MarshalResponse(want)
	if err != nil {
		t.Fatalf("MarshalResponse: %v", err)
	}

	got, err := UnmarshalResponse(encoded)
	if err != nil {
		t.Fatalf("UnmarshalResponse: %v", err)
	}
	if string(got.GetPayload()) != "payload" {
		t.Fatalf("payload: got %q", got.GetPayload())
	}
	if got.GetErrorText() != "failed" {
		t.Fatalf("errorText: got %q", got.GetErrorText())
	}
	if got.GetMetadata()["k"] != "v" {
		t.Fatalf("metadata: got %q", got.GetMetadata()["k"])
	}
}
