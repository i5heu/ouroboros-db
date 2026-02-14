package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

const (
	ctxNodeAdmissionV1  = "CTX_NODE_ADMISSION_V1"
	ctxNodePopV1        = "CTX_NODE_POP_V1"
	ctxNodeDelegationV1 = "CTX_NODE_DELEGATION_V1"
	ctxUserCAAnchorV1   = "CTX_USER_CA_ANCHOR_V1"
	currentCertVersion  = 2
)

// canonicalSerialize encodes a NodeCert into a
// deterministic binary representation:
// version(1) || len(KEM)(4) || KEM ||
// len(Sign)(4) || Sign || IssuerCAHash(32) ||
// ValidFrom(8, unix-sec BE) ||
// ValidUntil(8, unix-sec BE) ||
// Serial(16) || RoleClaims(1) || CertNonce(32).
func canonicalSerialize( // A
	cert *NodeCert,
) ([]byte, error) {
	kemBytes, err := cert.nodePubKey.MarshalBinaryKEM()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal KEM key: %w", err,
		)
	}

	signBytes, err := cert.nodePubKey.MarshalBinarySign()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal sign key: %w", err,
		)
	}

	if len(kemBytes) > math.MaxUint32 {
		return nil, fmt.Errorf(
			"KEM key too large: %d bytes",
			len(kemBytes),
		)
	}
	if len(signBytes) > math.MaxUint32 {
		return nil, fmt.Errorf(
			"sign key too large: %d bytes",
			len(signBytes),
		)
	}

	size := 1 + 4 + len(kemBytes) +
		4 + len(signBytes) +
		len(cert.issuerCAHash) +
		8 + 8 + 16 + 1 + 32
	buf := make([]byte, 0, size)

	buf = append(buf, cert.certVersion)

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(
		lenBuf[:], uint32(len(kemBytes)), //#nosec G115
	)
	buf = append(buf, lenBuf[:]...)
	buf = append(buf, kemBytes...)

	binary.BigEndian.PutUint32(
		lenBuf[:], uint32(len(signBytes)), //#nosec G115
	)
	buf = append(buf, lenBuf[:]...)
	buf = append(buf, signBytes...)

	buf = append(buf, cert.issuerCAHash[:]...)

	validFromUnix := cert.validFrom.Unix()
	if validFromUnix < 0 {
		return nil, errors.New(
			"validFrom must be unix epoch or later",
		)
	}
	validUntilUnix := cert.validUntil.Unix()
	if validUntilUnix < 0 {
		return nil, errors.New(
			"validUntil must be unix epoch or later",
		)
	}
	if cert.roleClaims < 0 || cert.roleClaims > 255 {
		return nil, errors.New(
			"roleClaims out of byte range",
		)
	}

	var timeBuf [8]byte
	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(validFromUnix),
	)
	buf = append(buf, timeBuf[:]...)

	binary.BigEndian.PutUint64(
		timeBuf[:],
		uint64(validUntilUnix),
	)
	buf = append(buf, timeBuf[:]...)

	buf = append(buf, cert.serial[:]...)

	buf = append(buf, byte(cert.roleClaims))

	buf = append(buf, cert.certNonce[:]...)

	return buf, nil
}

// signingPayload prepends the domain separation context
// to the canonical serialization of the certificate.
func signingPayload( // A
	cert *NodeCert,
) ([]byte, error) {
	canon, err := canonicalSerialize(cert)
	if err != nil {
		return nil, err
	}

	ctx := []byte(ctxNodeAdmissionV1)
	payload := make(
		[]byte, 0, len(ctx)+len(canon),
	)
	payload = append(payload, ctx...)
	payload = append(payload, canon...)

	return payload, nil
}

// popSigningPayload builds the signing payload for
// enrollment proof-of-possession.
func popSigningPayload( // A
	challenge []byte,
) []byte {
	ctx := []byte(ctxNodePopV1)
	payload := make(
		[]byte, 0, len(ctx)+len(challenge),
	)
	payload = append(payload, ctx...)
	payload = append(payload, challenge...)
	return payload
}

// delegationSigningPayload builds the domain-separated
// signing payload for a DelegationProof.
func delegationSigningPayload( // A
	proof *DelegationProof,
) ([]byte, error) {
	if proof == nil {
		return nil, errors.New(
			"delegation proof must not be nil",
		)
	}
	canon, err := canonicalSerializeDelegation(proof)
	if err != nil {
		return nil, err
	}
	ctx := []byte(ctxNodeDelegationV1)
	payload := make(
		[]byte, 0, len(ctx)+len(canon),
	)
	payload = append(payload, ctx...)
	payload = append(payload, canon...)
	return payload, nil
}

func userCAAnchorPayload( // PAP
	userCAPubKey *keys.PublicKey,
) ([]byte, error) {
	if userCAPubKey == nil {
		return nil, errors.New(
			"user CA public key must not be nil",
		)
	}
	signBytes, err := userCAPubKey.MarshalBinarySign()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal user CA sign key: %w", err,
		)
	}
	ctx := []byte(ctxUserCAAnchorV1)
	payload := make(
		[]byte, 0, len(ctx)+len(signBytes),
	)
	payload = append(payload, ctx...)
	payload = append(payload, signBytes...)
	return payload, nil
}

func SignUserCAAnchor( // PAP
	adminSigner *keys.AsyncCrypt,
	userCAPubKey *keys.PublicKey,
) ([]byte, error) {
	if adminSigner == nil {
		return nil, errors.New(
			"admin signer must not be nil",
		)
	}
	payload, err := userCAAnchorPayload(userCAPubKey)
	if err != nil {
		return nil, err
	}
	return adminSigner.Sign(payload)
}

func verifyUserCAAnchor( // PAP
	adminPubKey *keys.PublicKey,
	userCAPubKey *keys.PublicKey,
	anchorSig []byte,
) error {
	if adminPubKey == nil {
		return errors.New(
			"admin public key must not be nil",
		)
	}
	if len(anchorSig) == 0 {
		return errors.New(
			"anchor signature must not be empty",
		)
	}
	payload, err := userCAAnchorPayload(userCAPubKey)
	if err != nil {
		return err
	}
	if !adminPubKey.Verify(payload, anchorSig) {
		return errors.New(
			"user CA anchor signature verification failed",
		)
	}
	return nil
}

// SignNodeCert signs the domain-separated NodeCert
// payload with the provided CA signer.
func SignNodeCert( // A
	signer *keys.AsyncCrypt,
	cert *NodeCert,
) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("signer must not be nil")
	}
	payload, err := signingPayload(cert)
	if err != nil {
		return nil, err
	}
	return signer.Sign(payload)
}

// SignDelegationProof signs the domain-separated
// DelegationProof payload with the node signer.
func SignDelegationProof( // A
	signer *keys.AsyncCrypt,
	proof *DelegationProof,
) ([]byte, error) {
	if signer == nil {
		return nil, errors.New("signer must not be nil")
	}
	payload, err := delegationSigningPayload(proof)
	if err != nil {
		return nil, err
	}
	return signer.Sign(payload)
}

// computeCAHash derives the SHA-256 identifier for a CA
// from its signing public key bytes.
func computeCAHash( // A
	pub *keys.PublicKey,
) (CaHash, error) {
	signBytes, err := pub.MarshalBinarySign()
	if err != nil {
		return CaHash{}, fmt.Errorf(
			"marshal sign key: %w", err,
		)
	}
	return HashBytes(signBytes), nil
}

// verifyNodeCert validates a CA signature over the
// domain-separated NodeCert payload and returns the
// derived NodeID on success.
func verifyNodeCert( // AP
	caPubKey *keys.PublicKey,
	cert *NodeCert,
	caSignature []byte,
) (keys.NodeID, error) {
	if caPubKey == nil {
		return keys.NodeID{}, errors.New(
			"CA public key must not be nil",
		)
	}

	if cert == nil {
		return keys.NodeID{}, errors.New(
			"node cert must not be nil",
		)
	}

	if cert.NodePubKey() == nil {
		return keys.NodeID{}, errors.New(
			"node cert public key must not be nil",
		)
	}

	payload, err := signingPayload(cert)
	if err != nil {
		return keys.NodeID{}, fmt.Errorf(
			"build signing payload: %w", err,
		)
	}

	if !caPubKey.Verify(payload, caSignature) {
		return keys.NodeID{}, errors.New(
			"CA signature verification failed",
		)
	}

	return cert.NodeID()
}
