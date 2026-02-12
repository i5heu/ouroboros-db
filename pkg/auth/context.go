package auth

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

const (
	ctxNodeAdmissionV1 = "CTX_NODE_ADMISSION_V1"
	certVersion        = 1
)

// canonicalSerialize encodes a NodeCert into a
// deterministic binary representation:
// version(1) || len(KEM)(4) || KEM ||
// len(Sign)(4) || Sign || IssuerCAHash(32).
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
		4 + len(signBytes) + len(cert.issuerCAHash)
	buf := make([]byte, 0, size)

	buf = append(buf, certVersion)

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
