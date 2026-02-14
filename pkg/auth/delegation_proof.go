package auth

// DelegationProofImpl is a concrete implementation of
// the interfaces.DelegationProof interface.
type DelegationProofImpl struct { // A
	tlsCertPubKeyHash  []byte
	tlsExporterBinding []byte
	tlsTranscriptHash  []byte
	x509Fingerprint    []byte
	nodeCertBundleHash []byte
	notBefore          int64
	notAfter           int64
}

// NewDelegationProof constructs a DelegationProofImpl.
func NewDelegationProof( // A
	tlsCertPubKeyHash []byte,
	tlsExporterBinding []byte,
	tlsTranscriptHash []byte,
	x509Fingerprint []byte,
	nodeCertBundleHash []byte,
	notBefore int64,
	notAfter int64,
) *DelegationProofImpl {
	return &DelegationProofImpl{
		tlsCertPubKeyHash:  tlsCertPubKeyHash,
		tlsExporterBinding: tlsExporterBinding,
		tlsTranscriptHash:  tlsTranscriptHash,
		x509Fingerprint:    x509Fingerprint,
		nodeCertBundleHash: nodeCertBundleHash,
		notBefore:          notBefore,
		notAfter:           notAfter,
	}
}

// TLSCertPubKeyHash returns the hash of the TLS
// certificate's SubjectPublicKeyInfo.
func (d *DelegationProofImpl) TLSCertPubKeyHash() []byte { // A
	return d.tlsCertPubKeyHash
}

// TLSExporterBinding returns the TLS exporter value.
func (d *DelegationProofImpl) TLSExporterBinding() []byte { // A
	return d.tlsExporterBinding
}

// TLSTranscriptHash returns the TLS handshake
// transcript hash.
func (d *DelegationProofImpl) TLSTranscriptHash() []byte { // A
	return d.tlsTranscriptHash
}

// X509Fingerprint returns the X.509 certificate
// fingerprint.
func (d *DelegationProofImpl) X509Fingerprint() []byte { // A
	return d.x509Fingerprint
}

// NodeCertBundleHash returns the hash of the
// canonical NodeCert bundle.
func (d *DelegationProofImpl) NodeCertBundleHash() []byte { // A
	return d.nodeCertBundleHash
}

// NotBefore returns the delegation validity start.
func (d *DelegationProofImpl) NotBefore() int64 { // A
	return d.notBefore
}

// NotAfter returns the delegation validity end.
func (d *DelegationProofImpl) NotAfter() int64 { // A
	return d.notAfter
}
