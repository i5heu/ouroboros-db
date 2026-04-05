package auth

import (
	"bytes"
	"sort"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// cborEncMode is the RFC 8949 core deterministic
// encoding mode used for all canonical serialization.
var cborEncMode cbor.EncMode // A

func init() { // A
	var err error
	cborEncMode, err = cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(
			"failed to init CBOR enc mode: " + err.Error(),
		)
	}
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

// canonicalCert is the CBOR-serializable form of a
// NodeCert with deterministic field ordering.
type canonicalCert struct { // A
	CertVersion  uint16 `cbor:"1,keyasint"`
	NodePubKey   []byte `cbor:"2,keyasint"`
	IssuerCAHash string `cbor:"3,keyasint"`
	ValidFrom    int64  `cbor:"4,keyasint"`
	ValidUntil   int64  `cbor:"5,keyasint"`
	Serial       []byte `cbor:"6,keyasint"`
	CertNonce    []byte `cbor:"7,keyasint"`
}

// canonicalDelegation is the CBOR-serializable form
// of a DelegationProof.
type canonicalDelegation struct { // A
	TLSCertPubKeyHash  []byte `cbor:"1,keyasint"`
	TLSExporterBinding []byte `cbor:"2,keyasint"`
	TLSTranscriptHash  []byte `cbor:"3,keyasint"`
	X509Fingerprint    []byte `cbor:"4,keyasint"`
	NodeCertBundleHash []byte `cbor:"5,keyasint"`
	NotBefore          int64  `cbor:"6,keyasint"`
	NotAfter           int64  `cbor:"7,keyasint"`
}

// canonicalDelegationNoExporter omits
// TLSExporterBinding for use as TLS exporter context.
type canonicalDelegationNoExporter struct { // A
	TLSCertPubKeyHash  []byte `cbor:"1,keyasint"`
	TLSTranscriptHash  []byte `cbor:"3,keyasint"`
	X509Fingerprint    []byte `cbor:"4,keyasint"`
	NodeCertBundleHash []byte `cbor:"5,keyasint"`
	NotBefore          int64  `cbor:"6,keyasint"`
	NotAfter           int64  `cbor:"7,keyasint"`
}

// CanonicalNodeCert encodes a NodeCert into
// RFC 8949 deterministic CBOR.
func CanonicalNodeCert( // A
	cert NodeCertLike,
) ([]byte, error) {
	pubKey := cert.NodePubKey()
	nodePubKey, err := marshalPubKeyBytes(&pubKey)
	if err != nil {
		return nil, err
	}
	c := canonicalCert{
		CertVersion:  cert.CertVersion(),
		NodePubKey:   nodePubKey,
		IssuerCAHash: cert.IssuerCAHash(),
		ValidFrom:    cert.ValidFrom(),
		ValidUntil:   cert.ValidUntil(),
		Serial:       cert.Serial(),
		CertNonce:    cert.CertNonce(),
	}
	return cborEncMode.Marshal(c)
}

// CanonicalNodeCertBundle encodes a sorted array of
// NodeCerts into deterministic CBOR. Certs are sorted
// by their full canonical encodings to guarantee a
// total order even when partial fields collide.
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
	raw := make([]cbor.RawMessage, len(entries))
	for i, e := range entries {
		raw[i] = e.encoded
	}
	return cborEncMode.Marshal(raw)
}

// CanonicalDelegationProof encodes a DelegationProof
// into deterministic CBOR including all 7 fields.
func CanonicalDelegationProof( // A
	proof DelegationProofLike,
) ([]byte, error) {
	d := canonicalDelegation{
		TLSCertPubKeyHash:  proof.TLSCertPubKeyHash(),
		TLSExporterBinding: proof.TLSExporterBinding(),
		TLSTranscriptHash:  proof.TLSTranscriptHash(),
		X509Fingerprint:    proof.X509Fingerprint(),
		NodeCertBundleHash: proof.NodeCertBundleHash(),
		NotBefore:          proof.NotBefore(),
		NotAfter:           proof.NotAfter(),
	}
	return cborEncMode.Marshal(d)
}

// CanonicalDelegationProofForExporter encodes a
// DelegationProof without TLSExporterBinding, used as
// TLS exporter context.
func CanonicalDelegationProofForExporter( // A
	proof DelegationProofLike,
) ([]byte, error) {
	d := canonicalDelegationNoExporter{
		TLSCertPubKeyHash:  proof.TLSCertPubKeyHash(),
		TLSTranscriptHash:  proof.TLSTranscriptHash(),
		X509Fingerprint:    proof.X509Fingerprint(),
		NodeCertBundleHash: proof.NodeCertBundleHash(),
		NotBefore:          proof.NotBefore(),
		NotAfter:           proof.NotAfter(),
	}
	return cborEncMode.Marshal(d)
}

// DomainSeparate prepends a domain-separation context
// string to data: ctx || data.
func DomainSeparate( // A
	ctx string, data []byte,
) []byte {
	prefix := []byte(ctx)
	out := make([]byte, len(prefix)+len(data))
	copy(out, prefix)
	copy(out[len(prefix):], data)
	return out
}
