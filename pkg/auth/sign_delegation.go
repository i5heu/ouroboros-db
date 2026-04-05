package auth

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
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
//   - transcriptHash: the TLS handshake transcript
//     hash after TLS Finished. Obtain via:
//     conn.ConnectionState().TLSUnique or the
//     underlying hash from the TLS stack.
//   - exporterFn: closure over
//     tls.Conn.ExportKeyingMaterial; used to derive
//     the channel-bound exporter value with label
//     ExporterLabel.
//
// Returns the signed DelegationProof and the
// DelegationSig. Both must be sent to the verifier
// together with the cert bundle and CA signatures
// via writeAuthHandshake in pkg/carrier.
//
// Flow:
//
//  1. Compute CanonicalNodeCertBundle hash.
//  2. Build DelegationProof WITHOUT
//     TLSExporterBinding.
//  3. Derive exporter context =
//     CanonicalDelegationProofForExporter(proof).
//  4. Call exporterFn(ExporterLabel, ctx, 32) to
//     get the TLS exporter value.
//  5. Rebuild proof WITH TLSExporterBinding.
//  6. Canonical-encode + domain-separate.
//  7. Sign with the node's ML-DSA-87 private key.
func SignDelegation( // A
	nodeKey *keys.AsyncCrypt,
	certs []NodeCertLike,
	session *SessionIdentity,
	transcriptHash []byte,
	exporterFn ExporterFunc,
) (*DelegationProofImpl, []byte, error) {
	// 1. Bundle hash.
	bundleBytes, err := CanonicalNodeCertBundle(certs)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"canonical bundle: %w", err,
		)
	}
	bundleHash := sha256.Sum256(bundleBytes)

	now := time.Now().Unix()

	// 2. Build proof without exporter.
	proofNoExp := NewDelegationProof(
		session.CertPubKeyHash[:],
		nil, // exporter filled in step 5
		transcriptHash,
		session.X509Fingerprint[:],
		bundleHash[:],
		now,
		now+MaxDelegationTTL,
	)

	// 3. Exporter context.
	expCtx, err := CanonicalDelegationProofForExporter(
		proofNoExp,
	)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"exporter context: %w", err,
		)
	}

	// 4. Derive exporter value.
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

	// 5. Rebuild proof with exporter.
	proof := NewDelegationProof(
		session.CertPubKeyHash[:],
		exporter,
		transcriptHash,
		session.X509Fingerprint[:],
		bundleHash[:],
		now,
		now+MaxDelegationTTL,
	)

	// 6. Canonical-encode + domain-separate.
	canon, err := CanonicalDelegationProof(proof)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"canonical proof: %w", err,
		)
	}
	msg := DomainSeparate(CTXNodeDelegationV1, canon)

	// 7. Sign.
	sig, err := nodeKey.Sign(msg)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"sign delegation: %w", err,
		)
	}

	return proof, sig, nil
}
