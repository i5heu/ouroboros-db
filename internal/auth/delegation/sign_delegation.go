package delegation

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
)

// ExporterFunc derives a TLS keying-material export
// value. The standard Go TLS implementation provides
// this via tls.Conn.ExportKeyingMaterial. The carrier
// must pass a closure that calls:
//
//	conn.ConnectionState().ExportKeyingMaterial(
//	    label, context, length,
//	)
type ExporterFunc = func( // A
	label string, context []byte, length int,
) ([]byte, error)

// SignDelegation creates a DelegationProof binding
// the node's persistent ML-DSA-87 identity to the
// current TLS session and signs it. This implements
// the prover-side crypto of Phase 3 from auth.mmd.
//
// Parameters:
//
//   - nodeKey: the node's persistent ML-DSA-87 key
//     pair (same key whose public half appears in the
//     NodeCerts).
//   - certs: the node's NodeCert bundle (same certs
//     that will be sent to the verifier).
//   - session: the SessionIdentity from Phase 2
//     (NewSessionIdentity).
//   - exporterFn: closure over
//     tls.Conn.ExportKeyingMaterial; used to derive
//     both the transcript binding (via
//     TranscriptBindingLabel) and the channel-bound
//     exporter value (via ExporterLabel).
//
// Returns the signed DelegationProof and the
// DelegationSig. Both must be sent to the verifier
// together with the cert bundle and CA signatures
// via writeAuthHandshake in pkg/carrier.
//
// Flow:
//
//  1. Compute CanonicalNodeCertBundle hash.
//  2. Derive transcript binding via EKM.
//  3. Build DelegationProof WITHOUT
//     TLSExporterBinding.
//  4. Derive exporter context =
//     canonical.CanonicalDelegationProofForExporter(proof).
//  5. Call exporterFn(ExporterLabel, ctx, 32) to
//     get the TLS exporter value.
//  6. Rebuild proof WITH TLSExporterBinding.
//  7. Canonical-encode + domain-separate.
//  8. Sign with the node's ML-DSA-87 private key.
func SignDelegation( // A
	nodeKey *keys.AsyncCrypt,
	certs []canonical.NodeCertLike,
	session *SessionIdentity,
	exporterFn ExporterFunc,
) (*DelegationProofImpl, []byte, error) {
	// 1. Bundle hash.
	bundleBytes, err := canonical.CanonicalNodeCertBundle(certs)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"canonical bundle: %w", err,
		)
	}
	bundleHash := sha256.Sum256(bundleBytes)

	// 2. Derive transcript binding via EKM.
	// TLS 1.3 does not expose the raw transcript
	// hash; EKM provides equivalent binding per
	// RFC 9266.
	transcriptHash, err := exporterFn(
		TranscriptBindingLabel,
		nil,
		TLSTranscriptHashSize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"transcript binding: %w", err,
		)
	}

	now := time.Now().Unix()

	// 3. Build proof without exporter.
	proofNoExp := NewDelegationProof(
		session.CertPubKeyHash[:],
		nil, // exporter filled in step 6
		transcriptHash,
		session.X509Fingerprint[:],
		bundleHash[:],
		now,
		now+MaxDelegationTTL,
	)

	// 4. Exporter context.
	expCtx, err := canonical.CanonicalDelegationProofForExporter(
		proofNoExp,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"exporter context: %w", err,
		)
	}

	// 5. Derive exporter value.
	exporter, err := exporterFn(
		ExporterLabel,
		expCtx,
		TLSExporterBindingSize,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"export keying material: %w", err,
		)
	}

	// 6. Rebuild proof with exporter.
	proof := NewDelegationProof(
		session.CertPubKeyHash[:],
		exporter,
		transcriptHash,
		session.X509Fingerprint[:],
		bundleHash[:],
		now,
		now+MaxDelegationTTL,
	)

	// 7. Canonical-encode + domain-separate.
	canon, err := canonical.CanonicalDelegationProof(proof)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"canonical proof: %w", err,
		)
	}
	msg := canonical.DomainSeparate(CTXNodeDelegationV1, canon)

	// 8. Sign.
	sig, err := nodeKey.Sign(msg)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"sign delegation: %w", err,
		)
	}

	return proof, sig, nil
}
