package carrier

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
)

// ─── AUTH HANDSHAKE WIRE PROTOCOL ───
//
// This file implements the VERIFIER (receiver) side
// of the auth handshake wire protocol. The prover
// side (writeAuthHandshake) does NOT exist yet and
// must be implemented in the transport layer.
//
// Wire format (sent by the prover over a dedicated
// QUIC auth stream opened immediately after TLS
// Finished):
//
//   ┌────────────────────────────────────────┐
//   │  4 bytes: big-endian message length N  │
//   ├────────────────────────────────────────┤
//   │  N bytes: CBOR-encoded wireAuthMessage │
//   │    1: []wireNodeCert   (cert bundle)   │
//   │    2: [][]byte          (CA sigs)      │
//   │    3: wireDelegationProof              │
//   │    4: []byte            (delegation    │
//   │                          signature)    │
//   └────────────────────────────────────────┘
//
// The verifier calls readAuthHandshake() which:
//   1. Reads the length-prefixed CBOR message
//   2. Decodes wire types into auth.NodeCertImpl /
//      auth.DelegationProofImpl
//   3. Extracts TLS bindings from conn.TLSBindings()
//   4. Returns a PeerHandshake ready for
//      CarrierAuth.VerifyPeerCert()
//
// SECURITY NOTES:
//   - Message size capped at 1 MiB (maxAuthMessageSize)
//   - TLS bindings are read from the connection, NOT
//     from the wire message (prevents spoofing)
//   - The prover MUST NOT send TLS bindings; the
//     verifier derives them independently from the
//     transport
//
// TO IMPLEMENT writeAuthHandshake (prover side):
//
//   func writeAuthHandshake(
//       stream interfaces.Stream,
//       certs []auth.NodeCertLike,
//       caSigs [][]byte,
//       proof *auth.DelegationProofImpl,
//       delegationSig []byte,
//   ) error {
//       // 1. Convert certs to []wireNodeCert
//       // 2. Convert proof to wireDelegationProof
//       // 3. Build wireAuthMessage
//       // 4. CBOR-encode
//       // 5. Write 4-byte big-endian length
//       // 6. Write CBOR payload
//       // 7. Close stream
//   }
//

// maxAuthMessageSize limits handshake messages to
// 1 MiB to prevent resource exhaustion.
const maxAuthMessageSize = 1 << 20 // A

// wireNodeCert is the CBOR on-wire representation
// of a NodeCert used during the auth handshake.
type wireNodeCert struct { // A
	CertVersion  uint16 `cbor:"1,keyasint"`
	NodePubKey   []byte `cbor:"2,keyasint"`
	IssuerCAHash string `cbor:"3,keyasint"`
	ValidFrom    int64  `cbor:"4,keyasint"`
	ValidUntil   int64  `cbor:"5,keyasint"`
	Serial       []byte `cbor:"6,keyasint"`
	CertNonce    []byte `cbor:"7,keyasint"`
}

// wireDelegationProof is the CBOR on-wire
// representation of a DelegationProof.
type wireDelegationProof struct { // A
	TLSCertPubKeyHash  []byte `cbor:"1,keyasint"`
	TLSExporterBinding []byte `cbor:"2,keyasint"`
	TLSTranscriptHash  []byte `cbor:"3,keyasint"`
	X509Fingerprint    []byte `cbor:"4,keyasint"`
	NodeCertBundleHash []byte `cbor:"5,keyasint"`
	NotBefore          int64  `cbor:"6,keyasint"`
	NotAfter           int64  `cbor:"7,keyasint"`
}

// wireAuthMessage is the top-level handshake message
// sent by the prover over the auth stream.
type wireAuthMessage struct { // A
	Certs         []wireNodeCert      `cbor:"1,keyasint"`
	CASignatures  [][]byte            `cbor:"2,keyasint"`
	Delegation    wireDelegationProof `cbor:"3,keyasint"`
	DelegationSig []byte              `cbor:"4,keyasint"`
}

// readAuthHandshake reads a length-prefixed CBOR
// auth message from the stream and combines it with
// TLS bindings from the connection to produce a
// PeerHandshake.
func readAuthHandshake( // A
	stream interfaces.Stream,
	conn interfaces.Connection,
) (interfaces.PeerHandshake, error) {
	// Read 4-byte big-endian message length.
	var lenBuf [4]byte
	if _, err := io.ReadFull(
		stream, lenBuf[:],
	); err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf("read length: %w", err)
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen == 0 || msgLen > maxAuthMessageSize {
		return interfaces.PeerHandshake{},
			fmt.Errorf(
				"message size %d out of bounds",
				msgLen,
			)
	}

	// Read the CBOR payload.
	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, buf); err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf("read payload: %w", err)
	}

	var msg wireAuthMessage
	if err := cbor.Unmarshal(buf, &msg); err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf("decode handshake: %w", err)
	}

	// Convert wire certs to NodeCertLike.
	certs := make(
		[]auth.NodeCertLike, len(msg.Certs),
	)
	for i, wc := range msg.Certs {
		nc, err := wireToNodeCert(wc)
		if err != nil {
			return interfaces.PeerHandshake{},
				fmt.Errorf(
					"cert %d: %w", i, err,
				)
		}
		certs[i] = nc
	}

	// Convert wire delegation proof.
	proof := wireToDelegation(msg.Delegation)

	// Get TLS bindings from the transport.
	tls := conn.TLSBindings()

	return interfaces.PeerHandshake{
		Certs:           certs,
		CASignatures:    msg.CASignatures,
		DelegationProof: proof,
		DelegationSig:   msg.DelegationSig,
		TLS:             tls,
	}, nil
}

// wireToNodeCert converts a wire-format cert to a
// concrete NodeCertImpl.
func wireToNodeCert( // A
	wc wireNodeCert,
) (*auth.NodeCertImpl, error) {
	if len(wc.NodePubKey) <= auth.KEMPublicKeySize {
		return nil, fmt.Errorf(
			"public key too short: %d bytes",
			len(wc.NodePubKey),
		)
	}
	kemBytes := wc.NodePubKey[:auth.KEMPublicKeySize]
	signBytes := wc.NodePubKey[auth.KEMPublicKeySize:]
	pub, err := keys.NewPublicKeyFromBinary(
		kemBytes, signBytes,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"parse public key: %w", err,
		)
	}
	return auth.NewNodeCert(
		*pub,
		wc.IssuerCAHash,
		wc.ValidFrom,
		wc.ValidUntil,
		wc.Serial,
		wc.CertNonce,
	)
}

// wireToDelegation converts a wire-format delegation
// proof to a concrete DelegationProofImpl.
func wireToDelegation( // A
	wd wireDelegationProof,
) *auth.DelegationProofImpl {
	return auth.NewDelegationProof(
		wd.TLSCertPubKeyHash,
		wd.TLSExporterBinding,
		wd.TLSTranscriptHash,
		wd.X509Fingerprint,
		wd.NodeCertBundleHash,
		wd.NotBefore,
		wd.NotAfter,
	)
}
