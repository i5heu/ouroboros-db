package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// MarshalNodeCert serializes a NodeCert into its
// canonical wire form.
func MarshalNodeCert( // A
	cert *NodeCert,
) ([]byte, error) {
	if cert == nil {
		return nil, errors.New("node cert must not be nil")
	}
	return canonicalSerialize(cert)
}

// UnmarshalNodeCert parses a canonical NodeCert wire
// payload back into a NodeCert.
func UnmarshalNodeCert( // A
	data []byte,
) (*NodeCert, error) {
	kem, sign, offset, err := parseNodeCertKeyMaterial(data)
	if err != nil {
		return nil, err
	}

	parsed, offset, err := parseNodeCertTail(data, offset)
	if err != nil {
		return nil, err
	}
	if offset != len(data) {
		return nil, errors.New("node cert has trailing bytes")
	}

	pub, err := keys.NewPublicKeyFromBinary(kem, sign)
	if err != nil {
		return nil, fmt.Errorf("build public key: %w", err)
	}

	cert, err := NewNodeCert(NodeCertParams{
		NodePubKey:   pub,
		IssuerCAHash: parsed.issuer,
		ValidFrom:    time.Unix(parsed.validFromUnix, 0).UTC(),
		ValidUntil:   time.Unix(parsed.validUntilUnix, 0).UTC(),
		Serial:       parsed.serial,
		RoleClaims:   parsed.role,
		CertNonce:    parsed.nonce,
	})
	if err != nil {
		return nil, err
	}
	return cert, nil
}

type nodeCertTail struct { // A
	issuer         CaHash
	validFromUnix  int64
	validUntilUnix int64
	serial         [16]byte
	role           TrustScope
	nonce          [32]byte
}

func parseNodeCertKeyMaterial( // A
	data []byte,
) ([]byte, []byte, int, error) {
	if len(data) < 1+4+4+32+8+8+16+1+32 {
		return nil, nil, 0, errors.New("node cert payload too short")
	}
	offset := 0
	version := data[offset]
	offset++
	if version != currentCertVersion {
		return nil, nil, 0, fmt.Errorf(
			"unsupported node cert version: %d",
			version,
		)
	}
	kem, next, err := readSizedField(data, offset)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read KEM key: %w", err)
	}
	offset = next
	sign, next, err := readSizedField(data, offset)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read sign key: %w", err)
	}
	return kem, sign, next, nil
}

func parseNodeCertTail( // A
	data []byte,
	offset int,
) (nodeCertTail, int, error) {
	if len(data[offset:]) < 32+8+8+16+1+32 {
		return nodeCertTail{}, offset, errors.New(
			"node cert trailing fields too short",
		)
	}

	parsed := nodeCertTail{}
	copy(parsed.issuer[:], data[offset:offset+32])
	offset += 32

	validFromRaw := binary.BigEndian.Uint64(data[offset : offset+8])
	if validFromRaw > math.MaxInt64 {
		return nodeCertTail{}, offset, errors.New(
			"validFrom exceeds int64",
		)
	}
	parsed.validFromUnix = int64(validFromRaw)
	offset += 8

	validUntilRaw := binary.BigEndian.Uint64(data[offset : offset+8])
	if validUntilRaw > math.MaxInt64 {
		return nodeCertTail{}, offset, errors.New(
			"validUntil exceeds int64",
		)
	}
	parsed.validUntilUnix = int64(validUntilRaw)
	offset += 8

	copy(parsed.serial[:], data[offset:offset+16])
	offset += 16
	parsed.role = TrustScope(data[offset])
	offset++
	copy(parsed.nonce[:], data[offset:offset+32])
	offset += 32

	return parsed, offset, nil
}

// MarshalDelegationProof serializes a DelegationProof
// into its canonical wire form.
func MarshalDelegationProof( // A
	proof *DelegationProof,
) ([]byte, error) {
	if proof == nil {
		return nil, errors.New("delegation proof must not be nil")
	}
	return canonicalSerializeDelegation(proof)
}

// UnmarshalDelegationProof parses a canonical
// DelegationProof wire payload.
func UnmarshalDelegationProof( // A
	data []byte,
) (*DelegationProof, error) {
	if len(data) < 32+32+32+32+8+8+32 {
		return nil, errors.New("delegation proof payload too short")
	}
	offset := 0

	var certPubKeyHash [32]byte
	copy(certPubKeyHash[:], data[offset:offset+32])
	offset += 32

	var tlsExporter [32]byte
	copy(tlsExporter[:], data[offset:offset+32])
	offset += 32

	if len(data[offset:]) < 32+32+8+8+32 {
		return nil, errors.New("delegation trailing fields too short")
	}

	var x509FP [32]byte
	copy(x509FP[:], data[offset:offset+32])
	offset += 32

	var nodeHash CaHash
	copy(nodeHash[:], data[offset:offset+32])
	offset += 32

	notBeforeRaw := binary.BigEndian.Uint64(data[offset : offset+8])
	if notBeforeRaw > math.MaxInt64 {
		return nil, errors.New("notBefore exceeds int64")
	}
	notBeforeUnix := int64(notBeforeRaw)
	offset += 8
	notAfterRaw := binary.BigEndian.Uint64(data[offset : offset+8])
	if notAfterRaw > math.MaxInt64 {
		return nil, errors.New("notAfter exceeds int64")
	}
	notAfterUnix := int64(notAfterRaw)
	offset += 8

	var nonce [32]byte
	copy(nonce[:], data[offset:offset+32])
	offset += 32

	if offset != len(data) {
		return nil, errors.New("delegation proof has trailing bytes")
	}

	return NewDelegationProof(DelegationProofParams{
		TLSCertPubKeyHash: certPubKeyHash,
		TLSExporterBinding: tlsExporter,
		X509Fingerprint: x509FP,
		NodeCertHash:    nodeHash,
		NotBefore:       time.Unix(notBeforeUnix, 0).UTC(),
		NotAfter:        time.Unix(notAfterUnix, 0).UTC(),
		HandshakeNonce:  nonce,
	})
}

func readSizedField( // A
	data []byte,
	offset int,
) ([]byte, int, error) {
	if len(data[offset:]) < 4 {
		return nil, offset, errors.New("missing length")
	}
	fieldLen := int(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	if fieldLen < 0 || len(data[offset:]) < fieldLen {
		return nil, offset, errors.New("invalid field length")
	}
	field := make([]byte, fieldLen)
	copy(field, data[offset:offset+fieldLen])
	offset += fieldLen
	return field, offset, nil
}
