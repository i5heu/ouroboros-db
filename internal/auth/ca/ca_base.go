package ca

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	pb "github.com/i5heu/ouroboros-db/proto/auth"
)

// ErrInvalidCASignature is returned when a CA
// signature does not verify against the cert.
var ErrInvalidCASignature = errors.New( // A
	"invalid CA signature on certificate",
)

// CTXNodeAdmissionV1 is the domain-separation
// context for CA signing NodeCerts.
const CTXNodeAdmissionV1 = "OUROBOROS_NODE_ADMISSION_V1" // A

// CTXUserCAAnchorV1 is the domain-separation
// context for AdminCA anchoring UserCAs.
const CTXUserCAAnchorV1 = "OUROBOROS_USER_CA_ANCHOR_V1" // A

// newCABase constructs a caBase from concatenated
// KEM+Sign public key bytes.
func newCABase(pubKeyBytes []byte) (caBase, error) { // A
	pub, err := splitAndParsePubKey(pubKeyBytes)
	if err != nil {
		return caBase{}, err
	}
	h, err := caHash(pub)
	if err != nil {
		return caBase{}, err
	}
	cached, err := marshalPubKeyBytes(pub)
	if err != nil {
		return caBase{}, err
	}
	return caBase{pubKey: pub, pubKeyBytes: cached, hash: h}, nil
}

// caBase holds the shared public key and hash fields
// common to AdminCAImpl and UserCAImpl.
type caBase struct { // A
	pubKey      *keys.PublicKey
	pubKeyBytes []byte
	hash        string
}

// PubKey returns the concatenated KEM+Sign bytes.
func (b *caBase) PubKey() []byte { // A
	return bytes.Clone(b.pubKeyBytes)
}

// Hash returns hex(SHA-256(signPubKeyBytes)).
func (b *caBase) Hash() string { // A
	return b.hash
}

// Verify checks a signature against the CA's
// public key.
func (b *caBase) Verify(msg, sig []byte) bool { // A
	return b.pubKey.Verify(msg, sig)
}

// VerifyNodeCert verifies a CA signature on a
// NodeCert and returns the derived NodeID.
func (b *caBase) VerifyNodeCert( // A
	cert canonical.NodeCertLike,
	caSignature []byte,
) (keys.NodeID, error) {
	pubKey := cert.NodePubKey()
	nodePubKeyBytes, err := marshalPubKeyBytes(&pubKey)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"pubkey marshal failed: %w", err,
		)
	}
	c := &pb.CanonicalCert{
		CertVersion:  uint32(cert.CertVersion()),
		NodePubKey:   nodePubKeyBytes,
		IssuerCaHash: cert.IssuerCAHash(),
		ValidFrom:    cert.ValidFrom(),
		ValidUntil:   cert.ValidUntil(),
		Serial:       cert.Serial(),
		CertNonce:    cert.CertNonce(),
	}
	canonicalData, err := canonical.ProtoMarshalOpts.Marshal(c)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"canonical encoding failed: %w", err,
		)
	}
	msg := canonical.DomainSeparate(
		CTXNodeAdmissionV1, canonicalData,
	)
	if !b.pubKey.Verify(msg, caSignature) {
		return keys.NodeID{}, ErrInvalidCASignature
	}
	nid, err := pubKey.NodeID()
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"node ID derivation failed: %w", err,
		)
	}
	return nid, nil
}
