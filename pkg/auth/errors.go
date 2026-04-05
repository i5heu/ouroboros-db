package auth

import "errors"

// Sentinel errors for CarrierAuth verification
// failures.
var ( // A
	// ErrNoCerts is returned when the peer presents
	// an empty certificate bundle.
	ErrNoCerts = errors.New("no certificates presented")

	// ErrUnknownIssuer is returned when no certificate
	// has a known issuer in the trust store.
	ErrUnknownIssuer = errors.New(
		"no certificate has a known issuer",
	)

	// ErrCertExpired is returned when a certificate's
	// ValidUntil is in the past.
	ErrCertExpired = errors.New("certificate expired")

	// ErrCertNotYetValid is returned when a
	// certificate's ValidFrom is in the future.
	ErrCertNotYetValid = errors.New(
		"certificate not yet valid",
	)

	// ErrInvalidCASignature is returned when a CA
	// signature does not verify against the cert.
	ErrInvalidCASignature = errors.New(
		"invalid CA signature on certificate",
	)

	// ErrMismatchedNodeID is returned when certs in
	// the bundle bind to different NodeIDs.
	ErrMismatchedNodeID = errors.New(
		"certificates bind to different node IDs",
	)

	// ErrInvalidDelegationSig is returned when the
	// delegation proof signature fails verification.
	ErrInvalidDelegationSig = errors.New(
		"invalid delegation proof signature",
	)

	// ErrTLSBindingMismatch is returned when any TLS
	// binding field in the delegation proof does not
	// match the presented transport values.
	ErrTLSBindingMismatch = errors.New(
		"TLS binding mismatch in delegation proof",
	)

	// ErrInvalidBindingField is returned when a
	// cryptographic binding field has an invalid size.
	ErrInvalidBindingField = errors.New(
		"invalid delegation binding field",
	)

	// ErrBundleHashMismatch is returned when the
	// NodeCertBundleHash in the delegation proof does
	// not match the computed bundle hash.
	ErrBundleHashMismatch = errors.New(
		"node cert bundle hash mismatch",
	)

	// ErrDelegationExpired is returned when the
	// delegation proof's time window has elapsed.
	ErrDelegationExpired = errors.New(
		"delegation proof expired or not yet valid",
	)

	// ErrDelegationTooLong is returned when the
	// delegation proof's TTL exceeds MaxDelegationTTL.
	ErrDelegationTooLong = errors.New(
		"delegation proof TTL exceeds maximum",
	)

	// ErrNoValidCerts is returned when all certs are
	// filtered out by validity/revocation checks.
	ErrNoValidCerts = errors.New(
		"no valid certificates after filtering",
	)

	// ErrSignatureCountMismatch is returned when the
	// number of CA signatures does not match certs.
	ErrSignatureCountMismatch = errors.New(
		"certificate/signature count mismatch",
	)

	// ErrBundleTooLarge is returned when the peer cert
	// bundle exceeds MaxPeerCertBundleSize.
	ErrBundleTooLarge = errors.New(
		"peer cert bundle exceeds maximum size",
	)

	// ErrNilDelegationProof is returned when the
	// delegation proof is nil.
	ErrNilDelegationProof = errors.New(
		"delegation proof is nil",
	)

	// ErrNilCertEntry is returned when a nil entry
	// is found in the peer cert bundle.
	ErrNilCertEntry = errors.New(
		"nil entry in peer cert bundle",
	)

	// ErrCARevoked is returned when a CA has been
	// revoked.
	ErrCARevoked = errors.New("CA has been revoked")

	// ErrNodeRevoked is returned when a node has been
	// revoked.
	ErrNodeRevoked = errors.New(
		"node has been revoked",
	)

	// ErrCAAlreadyExists is returned when trying to
	// add a CA that already exists in the trust store.
	ErrCAAlreadyExists = errors.New(
		"CA already exists in trust store",
	)

	// ErrCANotFound is returned when a CA is not found
	// in the trust store.
	ErrCANotFound = errors.New(
		"CA not found in trust store",
	)

	// ErrInvalidAnchorSig is returned when a UserCA's
	// anchor signature does not verify.
	ErrInvalidAnchorSig = errors.New(
		"invalid anchor signature on user CA",
	)

	// ErrAnchorAdminNotFound is returned when the
	// AdminCA referenced by an anchor is not found.
	ErrAnchorAdminNotFound = errors.New(
		"anchor admin CA not found",
	)

	// ErrAnchorAdminRevoked is returned when the
	// AdminCA that anchored a UserCA has been revoked.
	ErrAnchorAdminRevoked = errors.New(
		"anchor admin CA has been revoked",
	)

	// ErrStatefulCert is returned when a cert returns
	// inconsistent values across reads (attack).
	ErrStatefulCert = errors.New(
		"stateful certificate detected",
	)

	// ErrUnsupportedCertVersion is returned when a
	// cert has an unknown version number.
	ErrUnsupportedCertVersion = errors.New(
		"unsupported certificate version",
	)

	// ErrRevocationStoreFull is returned when the
	// revocation map exceeds MaxRevocationEntries.
	ErrRevocationStoreFull = errors.New(
		"revocation store capacity exceeded",
	)
)

// AuthError wraps a sentinel error with runtime
// context while preserving errors.Is() compatibility.
type AuthError struct { // A
	Sentinel error
	Detail   string
	Context  map[string]any
}

// Error returns the sentinel message plus detail.
func (e *AuthError) Error() string { // A
	if e.Detail == "" {
		return e.Sentinel.Error()
	}
	return e.Sentinel.Error() + ": " + e.Detail
}

// Unwrap returns the sentinel for errors.Is().
func (e *AuthError) Unwrap() error { // A
	return e.Sentinel
}

// authErr creates a new AuthError with optional
// key-value context pairs.
func authErr( // A
	sentinel error,
	detail string,
	kvs ...any,
) *AuthError {
	var ctx map[string]any
	if len(kvs) > 1 {
		ctx = make(map[string]any, len(kvs)/2)
		for i := 0; i+1 < len(kvs); i += 2 {
			if k, ok := kvs[i].(string); ok {
				ctx[k] = kvs[i+1]
			}
		}
	}
	return &AuthError{
		Sentinel: sentinel,
		Detail:   detail,
		Context:  ctx,
	}
}
