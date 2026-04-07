package auth

import (
	"errors"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
)

func cloneBytes(src []byte) []byte { // A
	if src == nil {
		return nil
	}
	return append([]byte(nil), src...)
}

// certSnapshot is a frozen, immutable copy of a
// canonical.NodeCertLike. All interface methods are read once
// at construction; subsequent calls return the
// captured values. This prevents stateful-cert
// attacks where a malicious implementation changes
// return values between reads.
type certSnapshot struct { // A
	certVersion  uint16
	nodePubKey   keys.PublicKey
	issuerCAHash string
	validFrom    int64
	validUntil   int64
	serial       []byte
	certNonce    []byte
	nodeID       keys.NodeID
}

func (s *certSnapshot) CertVersion() uint16 { // A
	return s.certVersion
}

func (s *certSnapshot) NodePubKey() keys.PublicKey { // A
	return s.nodePubKey
}

func (s *certSnapshot) IssuerCAHash() string { // A
	return s.issuerCAHash
}

func (s *certSnapshot) ValidFrom() int64 { // A
	return s.validFrom
}

func (s *certSnapshot) ValidUntil() int64 { // A
	return s.validUntil
}

func (s *certSnapshot) Serial() []byte { // A
	return cloneBytes(s.serial)
}

func (s *certSnapshot) CertNonce() []byte { // A
	return cloneBytes(s.certNonce)
}

func (s *certSnapshot) NodeID() keys.NodeID { // A
	return s.nodeID
}

// snapshotCert reads all fields from a canonical.NodeCertLike
// once and verifies consistency: re-derives NodeID
// from NodePubKey and compares with NodeID(). If
// they differ, the cert is stateful/malicious.
func snapshotCert(c canonical.NodeCertLike) (*certSnapshot, error) { // A
	ver := c.CertVersion()
	if ver != DefaultCertVersion {
		return nil, authErr(
			ErrUnsupportedCertVersion,
			"unknown cert version",
			"version", ver,
		)
	}
	pub := c.NodePubKey()
	derivedNID, err := safeNodeID(pub)
	if err != nil {
		return nil, fmt.Errorf(
			"node ID derivation: %w", err,
		)
	}
	reportedNID := c.NodeID()
	if derivedNID != reportedNID {
		return nil, ErrStatefulCert
	}
	return &certSnapshot{
		certVersion:  c.CertVersion(),
		nodePubKey:   pub,
		issuerCAHash: c.IssuerCAHash(),
		validFrom:    c.ValidFrom(),
		validUntil:   c.ValidUntil(),
		serial:       cloneBytes(c.Serial()),
		certNonce:    cloneBytes(c.CertNonce()),
		nodeID:       derivedNID,
	}, nil
}

// snapshotCerts snapshots an entire cert bundle,
// returning an error if any cert is nil or stateful.
func snapshotCerts( // A
	certs []canonical.NodeCertLike,
) ([]*certSnapshot, error) {
	out := make([]*certSnapshot, len(certs))
	for i, c := range certs {
		if c == nil {
			return nil, authErr(
				ErrNilCertEntry,
				"nil entry in peer cert bundle",
				LogKeyCertIndex, i,
			)
		}
		snap, err := snapshotCert(c)
		if err != nil {
			return nil, authErr(
				err,
				"snapshot failed",
				LogKeyCertIndex, i,
			)
		}
		out[i] = snap
	}
	return out, nil
}

// safeNodeID derives a NodeID from a PublicKey,
// recovering from nil-pointer panics caused by
// zero-value or malformed public keys.
func safeNodeID( // A
	pub keys.PublicKey,
) (nid keys.NodeID, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(
				"invalid public key: nil dereference",
			)
		}
	}()
	return pub.NodeID()
}

// delegationSnapshot is a frozen copy of a
// canonical.DelegationProofLike.
type delegationSnapshot struct { // A
	tlsCertPubKeyHash  []byte
	tlsExporterBinding []byte
	tlsTranscriptHash  []byte
	x509Fingerprint    []byte
	nodeCertBundleHash []byte
	notBefore          int64
	notAfter           int64
}

func (d *delegationSnapshot) TLSCertPubKeyHash() []byte { // A
	return cloneBytes(d.tlsCertPubKeyHash)
}

func (d *delegationSnapshot) TLSExporterBinding() []byte { // A
	return cloneBytes(d.tlsExporterBinding)
}

func (d *delegationSnapshot) TLSTranscriptHash() []byte { // A
	return cloneBytes(d.tlsTranscriptHash)
}

func (d *delegationSnapshot) X509Fingerprint() []byte { // A
	return cloneBytes(d.x509Fingerprint)
}

func (d *delegationSnapshot) NodeCertBundleHash() []byte { // A
	return cloneBytes(d.nodeCertBundleHash)
}

func (d *delegationSnapshot) NotBefore() int64 { // A
	return d.notBefore
}

func (d *delegationSnapshot) NotAfter() int64 { // A
	return d.notAfter
}

// snapshotDelegation reads all fields from a
// canonical.DelegationProofLike once and returns a frozen copy.
func snapshotDelegation( // A
	p canonical.DelegationProofLike,
) *delegationSnapshot {
	return &delegationSnapshot{
		tlsCertPubKeyHash:  cloneBytes(p.TLSCertPubKeyHash()),
		tlsExporterBinding: cloneBytes(p.TLSExporterBinding()),
		tlsTranscriptHash:  cloneBytes(p.TLSTranscriptHash()),
		x509Fingerprint:    cloneBytes(p.X509Fingerprint()),
		nodeCertBundleHash: cloneBytes(p.NodeCertBundleHash()),
		notBefore:          p.NotBefore(),
		notAfter:           p.NotAfter(),
	}
}
