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
// This file implements BOTH sides of the auth
// handshake wire protocol:
//
//   - readAuthHandshake (VERIFIER/receiver side):
//     reads the length-prefixed CBOR message, decodes
//     wire types, combines with TLS bindings, and
//     returns a PeerHandshake for VerifyPeerCert.
//
//   - writeAuthHandshake (PROVER/sender side):
//     converts auth types to wire format, CBOR-
//     encodes, and writes the length-prefixed
//     message to the auth stream.
//
// Wire format (over a dedicated QUIC auth stream
// opened immediately after TLS Finished):
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
// SECURITY NOTES:
//   - Message size capped at 1 MiB (maxAuthMessageSize)
//   - TLS bindings are read from the connection, NOT
//     from the wire message (prevents spoofing)
//   - The prover MUST NOT send TLS bindings; the
//     verifier derives them independently from the
//     transport
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

	// Derive TLS bindings. Static values come from
	// the connection; exporter and transcript
	// bindings are derived via EKM using the proof
	// context so the verifier produces the same
	// values independently of the prover.
	tls := conn.TLSBindings()

	// Transcript binding via EKM (RFC 9266).
	transcript, err := conn.ExportKeyingMaterial(
		auth.TranscriptBindingLabel,
		nil,
		auth.TLSTranscriptHashSize,
	)
	if err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf(
				"derive transcript binding: %w",
				err,
			)
	}
	tls.TranscriptHash = transcript

	// Exporter binding uses the proof-minus-exporter
	// as EKM context, binding the exporter value to
	// the specific delegation proof.
	expCtx, err := auth.CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf(
				"exporter context: %w", err,
			)
	}
	exporter, err := conn.ExportKeyingMaterial(
		auth.ExporterLabel,
		expCtx,
		auth.TLSExporterBindingSize,
	)
	if err != nil {
		return interfaces.PeerHandshake{},
			fmt.Errorf(
				"derive exporter binding: %w",
				err,
			)
	}
	tls.ExporterBinding = exporter

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

// writeAuthHandshake encodes the prover's auth
// message and sends it over the stream as a
// length-prefixed CBOR payload. This is the
// counterpart of readAuthHandshake (verifier side).
//
// The caller (typically carrier.dialAndAuth) must:
//  1. Complete TLS and extract transcript/exporter.
//  2. Call auth.SignDelegation to get proof + sig.
//  3. Open a QUIC auth stream.
//  4. Call this function to send the message.
//  5. Close the stream.
func writeAuthHandshake( // A
	stream interfaces.Stream,
	certs []auth.NodeCertLike,
	caSigs [][]byte,
	proof *auth.DelegationProofImpl,
	delegationSig []byte,
) error {
	// Convert certs to wire format.
	wireCerts := make(
		[]wireNodeCert, len(certs),
	)
	for i, c := range certs {
		wc, err := certToWire(c)
		if err != nil {
			return fmt.Errorf(
				"cert %d: %w", i, err,
			)
		}
		wireCerts[i] = wc
	}

	msg := wireAuthMessage{
		Certs:        wireCerts,
		CASignatures: caSigs,
		Delegation: wireDelegationProof{
			TLSCertPubKeyHash:  proof.TLSCertPubKeyHash(),
			TLSExporterBinding: proof.TLSExporterBinding(),
			TLSTranscriptHash:  proof.TLSTranscriptHash(),
			X509Fingerprint:    proof.X509Fingerprint(),
			NodeCertBundleHash: proof.NodeCertBundleHash(),
			NotBefore:          proof.NotBefore(),
			NotAfter:           proof.NotAfter(),
		},
		DelegationSig: delegationSig,
	}

	payload, err := cbor.Marshal(msg)
	if err != nil {
		return fmt.Errorf("encode handshake: %w", err)
	}

	if len(payload) > maxAuthMessageSize {
		return fmt.Errorf(
			"auth message too large: %d bytes",
			len(payload),
		)
	}

	// Write 4-byte big-endian length prefix.
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(
		lenBuf[:], uint32(len(payload)),
	)
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if _, err := stream.Write(payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}

// certToWire converts a NodeCertLike to the on-wire
// CBOR representation.
func certToWire( // A
	c auth.NodeCertLike,
) (wireNodeCert, error) {
	pub := c.NodePubKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		return wireNodeCert{}, fmt.Errorf(
			"marshal public key: %w", err,
		)
	}
	return wireNodeCert{
		CertVersion:  c.CertVersion(),
		NodePubKey:   pubBytes,
		IssuerCAHash: c.IssuerCAHash(),
		ValidFrom:    c.ValidFrom(),
		ValidUntil:   c.ValidUntil(),
		Serial:       c.Serial(),
		CertNonce:    c.CertNonce(),
	}, nil
}
