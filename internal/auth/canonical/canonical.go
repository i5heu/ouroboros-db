package canonical

import (
	"bytes"
	"sort"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	pb "github.com/i5heu/ouroboros-db/proto/auth"
	"google.golang.org/protobuf/proto"
)

// ProtoMarshalOpts uses deterministic marshaling for
// all canonical serialization to ensure reproducible
// byte output across platforms.
var ProtoMarshalOpts = proto.MarshalOptions{ // A
	Deterministic: true,
}

// NodeCertLike defines the methods needed for
// canonical NodeCert serialization. Structurally
// matches interfaces.NodeCert without importing it.
type NodeCertLike interface { // A
	CertVersion() uint16
	NodePubKey() keys.PublicKey
	IssuerCAHash() string
	ValidFrom() int64
	ValidUntil() int64
	Serial() []byte
	CertNonce() []byte
	NodeID() keys.NodeID
}

// DelegationProofLike defines the methods needed for
// canonical DelegationProof serialization.
type DelegationProofLike interface { // A
	TLSCertPubKeyHash() []byte
	TLSExporterBinding() []byte
	TLSTranscriptHash() []byte
	X509Fingerprint() []byte
	NodeCertBundleHash() []byte
	NotBefore() int64
	NotAfter() int64
}

// marshalPubKeyBytes returns concatenated KEM+Sign
// binary representations of a public key.
func marshalPubKeyBytes( // A
	pub *keys.PublicKey,
) ([]byte, error) {
	kem, err := pub.MarshalBinaryKEM()
	if err != nil {
		return nil, err
	}
	sign, err := pub.MarshalBinarySign()
	if err != nil {
		return nil, err
	}
	out := make([]byte, len(kem)+len(sign))
	copy(out, kem)
	copy(out[len(kem):], sign)
	return out, nil
}

// CanonicalNodeCert encodes a NodeCert into
// deterministic protobuf.
func CanonicalNodeCert( // A
	cert NodeCertLike,
) ([]byte, error) {
	pubKey := cert.NodePubKey()
	nodePubKey, err := marshalPubKeyBytes(&pubKey)
	if err != nil {
		return nil, err
	}
	c := &pb.CanonicalCert{
		CertVersion:  uint32(cert.CertVersion()),
		NodePubKey:   nodePubKey,
		IssuerCaHash: cert.IssuerCAHash(),
		ValidFrom:    cert.ValidFrom(),
		ValidUntil:   cert.ValidUntil(),
		Serial:       cert.Serial(),
		CertNonce:    cert.CertNonce(),
	}
	return ProtoMarshalOpts.Marshal(c)
}

// CanonicalNodeCertBundle encodes a sorted array of
// NodeCerts into deterministic protobuf. Certs are
// sorted by their full canonical encodings to
// guarantee a total order even when partial fields
// collide.
func CanonicalNodeCertBundle( // A
	certs []NodeCertLike,
) ([]byte, error) {
	type sortEntry struct {
		encoded []byte
	}
	entries := make([]sortEntry, len(certs))
	for i, cert := range certs {
		enc, err := CanonicalNodeCert(cert)
		if err != nil {
			return nil, err
		}
		entries[i] = sortEntry{
			encoded: enc,
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(
			entries[i].encoded,
			entries[j].encoded,
		) < 0
	})
	bundle := &pb.CanonicalCertBundle{}
	for _, e := range entries {
		bundle.Certs = append(bundle.Certs, e.encoded)
	}
	return ProtoMarshalOpts.Marshal(bundle)
}

// CanonicalDelegationProof encodes a DelegationProof
// into deterministic protobuf including all 7 fields.
func CanonicalDelegationProof( // A
	proof DelegationProofLike,
) ([]byte, error) {
	d := &pb.CanonicalDelegation{
		TlsCertPubKeyHash:  proof.TLSCertPubKeyHash(),
		TlsExporterBinding: proof.TLSExporterBinding(),
		TlsTranscriptHash:  proof.TLSTranscriptHash(),
		X509Fingerprint:    proof.X509Fingerprint(),
		NodeCertBundleHash: proof.NodeCertBundleHash(),
		NotBefore:          proof.NotBefore(),
		NotAfter:           proof.NotAfter(),
	}
	return ProtoMarshalOpts.Marshal(d)
}

// CanonicalDelegationProofForExporter encodes a
// DelegationProof without TLSExporterBinding, used as
// TLS exporter context.
func CanonicalDelegationProofForExporter( // A
	proof DelegationProofLike,
) ([]byte, error) {
	d := &pb.CanonicalDelegationNoExporter{
		TlsCertPubKeyHash:  proof.TLSCertPubKeyHash(),
		TlsTranscriptHash:  proof.TLSTranscriptHash(),
		X509Fingerprint:    proof.X509Fingerprint(),
		NodeCertBundleHash: proof.NodeCertBundleHash(),
		NotBefore:          proof.NotBefore(),
		NotAfter:           proof.NotAfter(),
	}
	return ProtoMarshalOpts.Marshal(d)
}

// DomainSeparate prepends a length-prefixed
// domain-separation context to data:
// bigEndian(len(ctx)) || ctx || data.
// The 4-byte length prefix removes ambiguity when
// context strings have variable lengths.
func DomainSeparate( // A
	ctx string, data []byte,
) []byte {
	prefix := []byte(ctx)
	l := len(prefix)
	out := make([]byte, 4+l+len(data))
	out[0] = byte(l >> 24) //nolint:gosec // G115: bit-shifted value always fits in byte
	out[1] = byte(l >> 16) //nolint:gosec // G115: bit-shifted value always fits in byte
	out[2] = byte(l >> 8)  //nolint:gosec // G115: bit-shifted value always fits in byte
	out[3] = byte(l)       //nolint:gosec // G115: bit-shifted value always fits in byte
	copy(out[4:], prefix)
	copy(out[4+l:], data)
	return out
}
