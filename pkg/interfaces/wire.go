package interfaces

import (
	"fmt"

	oldproto "github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/protoadapt"
)

// WireMessage is the default protobuf envelope used by
// carrier and existing packages while higher-level
// message schemas are still evolving.
type WireMessage struct { // A
	Type    int32  `protobuf:"varint,1,opt,name=type,proto3" json:"type,omitempty"`
	Payload []byte `protobuf:"bytes,2,opt,name=payload,proto3" json:"payload,omitempty"`
}

// NewWireMessage creates a default protobuf message
// envelope from the given type and payload bytes.
func NewWireMessage( // A
	msgType MessageType,
	payload []byte,
) *WireMessage {
	cloned := append([]byte(nil), payload...)
	return &WireMessage{
		Type:    int32(msgType),
		Payload: cloned,
	}
}

// Reset resets the message state.
func (m *WireMessage) Reset() { // A
	*m = WireMessage{}
}

// String returns the protobuf string form.
func (m *WireMessage) String() string { // A
	return oldproto.CompactTextString(m)
}

// ProtoMessage marks WireMessage as protobuf.
func (*WireMessage) ProtoMessage() {} // A

// GetType returns the message type discriminator.
func (m *WireMessage) GetType() int32 { // A
	if m == nil {
		return 0
	}
	return m.Type
}

// GetPayload returns the encoded message payload.
func (m *WireMessage) GetPayload() []byte { // A
	if m == nil {
		return nil
	}
	return append([]byte(nil), m.Payload...)
}

// WireResponse is the default protobuf response
// envelope used by carrier and existing packages.
type WireResponse struct { // A
	Payload   []byte            `protobuf:"bytes,1,opt,name=payload,proto3" json:"payload,omitempty"`
	ErrorText string            `protobuf:"bytes,2,opt,name=errorText,proto3" json:"errorText,omitempty"`
	Metadata  map[string]string `protobuf:"bytes,3,rep,name=metadata,proto3" json:"metadata,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

// NewWireResponse creates a default protobuf response
// envelope from handler results.
func NewWireResponse( // A
	payload []byte,
	errorText string,
	metadata map[string]string,
) *WireResponse {
	clonedPayload := append([]byte(nil), payload...)
	clonedMeta := make(map[string]string, len(metadata))
	for key, value := range metadata {
		clonedMeta[key] = value
	}
	return &WireResponse{
		Payload:   clonedPayload,
		ErrorText: errorText,
		Metadata:  clonedMeta,
	}
}

// Reset resets the response state.
func (r *WireResponse) Reset() { // A
	*r = WireResponse{}
}

// String returns the protobuf string form.
func (r *WireResponse) String() string { // A
	return oldproto.CompactTextString(r)
}

// ProtoMessage marks WireResponse as protobuf.
func (*WireResponse) ProtoMessage() {} // A

// GetPayload returns the encoded response payload.
func (r *WireResponse) GetPayload() []byte { // A
	if r == nil {
		return nil
	}
	return append([]byte(nil), r.Payload...)
}

// GetErrorText returns the response error text.
func (r *WireResponse) GetErrorText() string { // A
	if r == nil {
		return ""
	}
	return r.ErrorText
}

// GetMetadata returns the response metadata.
func (r *WireResponse) GetMetadata() map[string]string { // A
	if r == nil {
		return nil
	}
	cloned := make(map[string]string, len(r.Metadata))
	for key, value := range r.Metadata {
		cloned[key] = value
	}
	return cloned
}

// MarshalMessage encodes a protobuf transport message.
func MarshalMessage( // A
	msg Message,
) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message must not be nil")
	}
	v1, ok := msg.(protoadapt.MessageV1)
	if !ok {
		return nil, fmt.Errorf("message must implement proto v1")
	}
	return proto.Marshal(protoadapt.MessageV2Of(v1))
}

// UnmarshalMessage decodes a protobuf transport
// message into the default wire envelope.
func UnmarshalMessage( // A
	data []byte,
) (*WireMessage, error) {
	var msg WireMessage
	if err := proto.Unmarshal(data, protoadapt.MessageV2Of(&msg)); err != nil {
		return nil, err
	}
	return &msg, nil
}

// MarshalResponse encodes a protobuf response.
func MarshalResponse( // A
	resp Response,
) ([]byte, error) {
	if resp == nil {
		return nil, fmt.Errorf("response must not be nil")
	}
	v1, ok := resp.(protoadapt.MessageV1)
	if !ok {
		return nil, fmt.Errorf("response must implement proto v1")
	}
	return proto.Marshal(protoadapt.MessageV2Of(v1))
}

// UnmarshalResponse decodes a protobuf response into
// the default wire response envelope.
func UnmarshalResponse( // A
	data []byte,
) (*WireResponse, error) {
	var resp WireResponse
	if err := proto.Unmarshal(data, protoadapt.MessageV2Of(&resp)); err != nil {
		return nil, err
	}
	return &resp, nil
}
