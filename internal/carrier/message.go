package carrier

import (
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
)

// Message represents a message exchanged between nodes in the cluster.
type Message struct { // A
	// Type identifies what kind of message this is.
	Type MessageType
	// Payload is the message data, format depends on Type.
	Payload []byte
}

// EncryptedMessage represents a message encrypted for a specific recipient.
type EncryptedMessage struct { // A
	// SenderID identifies the sender of the message.
	SenderID NodeID
	// Encrypted contains the encrypted payload.
	Encrypted *encrypt.EncryptResult
	// Signature is the sender's signature over the encrypted data.
	Signature []byte
}

// BroadcastResult contains the result of a broadcast operation.
type BroadcastResult struct { // A
	// SuccessNodes are nodes that successfully received the message.
	SuccessNodes []Node
	// FailedNodes maps failed nodes to their error.
	FailedNodes map[NodeID]error
}
