package transport

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/internal/auth/delegation"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
)

// ─── AUTH HANDSHAKE WIRE PROTOCOL ───
//
// This file implements BOTH sides of the auth
// handshake wire protocol:
//
//   - readAuthHandshake (VERIFIER/receiver side):
//     reads the length-prefixed protobuf message,
//     decodes wire types, combines with TLS bindings,
//     and returns a PeerHandshake for VerifyPeerCert.
//
//   - writeAuthHandshake (PROVER/sender side):
//     converts auth types to wire format, protobuf-
//     encodes, and writes the length-prefixed
//     message to the auth stream.
//
// Wire format (over a dedicated QUIC auth stream
// opened immediately after TLS Finished):
//
//   ┌────────────────────────────────────────┐
//   │  4 bytes: big-endian message length N  │
//   ├────────────────────────────────────────┤
//   │  N bytes: protobuf WireAuthMessage     │
//   │    1: []WireNodeCert   (cert bundle)   │
//   │    2: []bytes           (CA sigs)      │
//   │    3: []WireEmbeddedCA                 │
//   │    4: WireDelegationProof              │
//   │    5: bytes             (delegation    │
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

// readAuthHandshake reads a length-prefixed protobuf
// auth message from the stream and combines it with
// TLS bindings from the connection to produce a
// PeerHandshake.
func readAuthHandshake( // A
	stream interfaces.Stream,
	conn interfaces.Connection,
) (interfaces.PeerHandshake, error) {
	msg, err := readWireMessage(stream)
	if err != nil {
		return interfaces.PeerHandshake{}, err
	}

	certs, err := parseWireCerts(msg)
	if err != nil {
		return interfaces.PeerHandshake{}, err
	}

	authorities := make(
		[]auth.EmbeddedCA, len(msg.Authorities),
	)
	for i, a := range msg.Authorities {
		authorities[i] = auth.EmbeddedCA{
			Type: a.Type,
			PubKEM: append(
				[]byte(nil), a.PubKem...,
			),
			PubSign: append(
				[]byte(nil), a.PubSign...,
			),
			AnchorSig: append(
				[]byte(nil), a.AnchorSig...,
			),
			AnchorAdmin: a.AnchorAdmin,
		}
	}

	proof := pbToDelegation(msg.Delegation)
	tls, err := derivePeerTLSBindings(
		conn, proof,
	)
	if err != nil {
		return interfaces.PeerHandshake{}, err
	}

	return interfaces.PeerHandshake{
		Certs:           certs,
		CASignatures:    msg.CaSignatures,
		Authorities:     authorities,
		DelegationProof: proof,
		DelegationSig:   msg.DelegationSig,
		TLS:             tls,
	}, nil
}

func readWireMessage( // A
	stream interfaces.Stream,
) (*pb.WireAuthMessage, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(
		stream, lenBuf[:],
	); err != nil {
		return nil, fmt.Errorf(
			"read length: %w", err,
		)
	}
	msgLen := binary.BigEndian.Uint32(lenBuf[:])
	if msgLen == 0 || msgLen > maxAuthMessageSize {
		return nil, fmt.Errorf(
			"message size %d out of bounds",
			msgLen,
		)
	}

	buf := make([]byte, msgLen)
	if _, err := io.ReadFull(
		stream, buf,
	); err != nil {
		return nil, fmt.Errorf(
			"read payload: %w", err,
		)
	}

	var msg pb.WireAuthMessage
	if err := proto.Unmarshal(
		buf, &msg,
	); err != nil {
		return nil, fmt.Errorf(
			"decode handshake: %w", err,
		)
	}
	return &msg, nil
}

func parseWireCerts( // A
	msg *pb.WireAuthMessage,
) ([]canonical.NodeCertLike, error) {
	certs := make(
		[]canonical.NodeCertLike, len(msg.Certs),
	)
	for i, wc := range msg.Certs {
		nc, err := pbToNodeCert(wc)
		if err != nil {
			return nil, fmt.Errorf(
				"cert %d: %w", i, err,
			)
		}
		certs[i] = nc
	}
	return certs, nil
}

func derivePeerTLSBindings( // A
	conn interfaces.Connection,
	proof *delegation.DelegationProofImpl,
) (interfaces.TLSBindings, error) {
	tls := conn.TLSBindings()

	transcript, err := conn.ExportKeyingMaterial(
		delegation.TranscriptBindingLabel,
		nil,
		delegation.TLSTranscriptHashSize,
	)
	if err != nil {
		return interfaces.TLSBindings{},
			fmt.Errorf(
				"derive transcript binding: %w",
				err,
			)
	}
	tls.TranscriptHash = transcript

	expCtx, err := canonical.CanonicalDelegationProofForExporter(
		proof,
	)
	if err != nil {
		return interfaces.TLSBindings{},
			fmt.Errorf(
				"exporter context: %w", err,
			)
	}
	exporter, err := conn.ExportKeyingMaterial(
		delegation.ExporterLabel,
		expCtx,
		delegation.TLSExporterBindingSize,
	)
	if err != nil {
		return interfaces.TLSBindings{},
			fmt.Errorf(
				"derive exporter binding: %w",
				err,
			)
	}
	tls.ExporterBinding = exporter
	return tls, nil
}

// pbToNodeCert converts a protobuf WireNodeCert to a
// concrete NodeCertImpl.
func pbToNodeCert( // A
	wc *pb.WireNodeCert,
) (*certpkg.NodeCertImpl, error) {
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
	return certpkg.NewNodeCert(
		*pub,
		wc.IssuerCaHash,
		wc.ValidFrom,
		wc.ValidUntil,
		wc.Serial,
		wc.CertNonce,
	)
}

// pbToDelegation converts a protobuf WireDelegation-
// Proof to a concrete DelegationProofImpl.
func pbToDelegation( // A
	wd *pb.WireDelegationProof,
) *delegation.DelegationProofImpl {
	if wd == nil {
		return delegation.NewDelegationProof(
			nil, nil, nil, nil, nil, 0, 0,
		)
	}
	return delegation.NewDelegationProof(
		wd.TlsCertPubKeyHash,
		wd.TlsExporterBinding,
		wd.TlsTranscriptHash,
		wd.X509Fingerprint,
		wd.NodeCertBundleHash,
		wd.NotBefore,
		wd.NotAfter,
	)
}

// writeAuthHandshake encodes the prover's auth
// message and sends it over the stream as a
// length-prefixed protobuf payload. This is the
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
	certs []canonical.NodeCertLike,
	caSigs [][]byte,
	authorities []auth.EmbeddedCA,
	proof *delegation.DelegationProofImpl,
	delegationSig []byte,
) error {
	// Convert certs to wire format.
	wireCerts := make(
		[]*pb.WireNodeCert, len(certs),
	)
	for i, c := range certs {
		wc, err := certToPB(c)
		if err != nil {
			return fmt.Errorf(
				"cert %d: %w", i, err,
			)
		}
		wireCerts[i] = wc
	}

	wireAuthorities := make(
		[]*pb.WireEmbeddedCA, len(authorities),
	)
	for i, a := range authorities {
		wireAuthorities[i] = &pb.WireEmbeddedCA{
			Type:        a.Type,
			PubKem:      append([]byte(nil), a.PubKEM...),
			PubSign:     append([]byte(nil), a.PubSign...),
			AnchorSig:   append([]byte(nil), a.AnchorSig...),
			AnchorAdmin: a.AnchorAdmin,
		}
	}

	msg := &pb.WireAuthMessage{
		Certs:        wireCerts,
		CaSignatures: caSigs,
		Authorities:  wireAuthorities,
		Delegation: &pb.WireDelegationProof{
			TlsCertPubKeyHash:  proof.TLSCertPubKeyHash(),
			TlsExporterBinding: proof.TLSExporterBinding(),
			TlsTranscriptHash:  proof.TLSTranscriptHash(),
			X509Fingerprint:    proof.X509Fingerprint(),
			NodeCertBundleHash: proof.NodeCertBundleHash(),
			NotBefore:          proof.NotBefore(),
			NotAfter:           proof.NotAfter(),
		},
		DelegationSig: delegationSig,
	}

	payload, err := proto.Marshal(msg)
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
	//#nosec G115 // safe: payload size checked against maxAuthMessageSize
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

// certToPB converts a NodeCertLike to the protobuf
// wire representation.
func certToPB( // A
	c canonical.NodeCertLike,
) (*pb.WireNodeCert, error) {
	pub := c.NodePubKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal public key: %w", err,
		)
	}
	return &pb.WireNodeCert{
		CertVersion:  uint32(c.CertVersion()),
		NodePubKey:   pubBytes,
		IssuerCaHash: c.IssuerCAHash(),
		ValidFrom:    c.ValidFrom(),
		ValidUntil:   c.ValidUntil(),
		Serial:       c.Serial(),
		CertNonce:    c.CertNonce(),
	}, nil
}
