package auth

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestCanonicalSerializeDeterministic( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)

	b1, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize 1: %v", err)
	}

	b2, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize 2: %v", err)
	}

	if !bytes.Equal(b1, b2) {
		t.Error(
			"canonicalSerialize not deterministic",
		)
	}
}

func TestCanonicalSerializeDifferentCerts( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	node1AC := generateKeys(t)
	node2AC := generateKeys(t)

	cert1 := buildTestCert(
		t, caPub, pubKeyPtr(t, node1AC), ScopeAdmin,
	)
	cert2 := buildTestCert(
		t, caPub, pubKeyPtr(t, node2AC), ScopeAdmin,
	)

	b1, _ := canonicalSerialize(cert1)
	b2, _ := canonicalSerialize(cert2)

	if bytes.Equal(b1, b2) {
		t.Error(
			"different certs should produce " +
				"different serializations",
		)
	}
}

func TestCanonicalSerializeStartsWithVersion( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeUser,
	)

	b, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize: %v", err)
	}

	if b[0] != currentCertVersion {
		t.Errorf(
			"first byte = %d, want %d",
			b[0], currentCertVersion,
		)
	}
}

func TestCanonicalSerializeEndsWithCertNonce( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)

	b, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize: %v", err)
	}

	suffix := b[len(b)-32:]
	nonce := cert.CertNonce()
	if !bytes.Equal(suffix, nonce[:]) {
		t.Error(
			"serialization should end with " +
				"CertNonce",
		)
	}
}

//nolint:cyclop // exhaustive byte-order assertions are intentionally linear.
func TestCanonicalSerializeFieldOrdering( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)

	b, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize: %v", err)
	}

	// version(1)
	offset := 0
	if b[offset] != currentCertVersion {
		t.Errorf(
			"version at offset 0: got %d, want %d",
			b[offset], currentCertVersion,
		)
	}
	offset++

	// len(KEM)(4) || KEM
	kemLen := binary.BigEndian.Uint32(
		b[offset : offset+4],
	)
	offset += 4 + int(kemLen)

	// len(Sign)(4) || Sign
	signLen := binary.BigEndian.Uint32(
		b[offset : offset+4],
	)
	offset += 4 + int(signLen)

	// IssuerCAHash(32)
	caHash := cert.IssuerCAHash()
	if !bytes.Equal(
		b[offset:offset+32], caHash[:],
	) {
		t.Error("IssuerCAHash mismatch at expected offset")
	}
	offset += 32

	// ValidFrom(8, BE)
	vfUnix := binary.BigEndian.Uint64(
		b[offset : offset+8],
	)
	expectedVF := cert.ValidFrom().Unix()
	if expectedVF < 0 {
		t.Fatalf("validFrom unix must be non-negative")
	}
	if vfUnix != uint64(expectedVF) {
		t.Errorf(
			"validFrom: got %d, want %d",
			vfUnix, expectedVF,
		)
	}
	offset += 8

	// ValidUntil(8, BE)
	vuUnix := binary.BigEndian.Uint64(
		b[offset : offset+8],
	)
	expectedVU := cert.ValidUntil().Unix()
	if expectedVU < 0 {
		t.Fatalf("validUntil unix must be non-negative")
	}
	if vuUnix != uint64(expectedVU) {
		t.Errorf(
			"validUntil: got %d, want %d",
			vuUnix, expectedVU,
		)
	}
	offset += 8

	// Serial(16)
	serial := cert.Serial()
	if !bytes.Equal(b[offset:offset+16], serial[:]) {
		t.Error("serial mismatch at expected offset")
	}
	offset += 16

	// RoleClaims(1)
	if int(b[offset]) != int(cert.RoleClaims()) {
		t.Errorf(
			"roleClaims: got %d, want %d",
			b[offset], cert.RoleClaims(),
		)
	}
	offset++

	// CertNonce(32)
	nonce := cert.CertNonce()
	if !bytes.Equal(b[offset:offset+32], nonce[:]) {
		t.Error("certNonce mismatch at expected offset")
	}
	offset += 32

	if offset != len(b) {
		t.Errorf(
			"unexpected trailing bytes: consumed %d, total %d",
			offset, len(b),
		)
	}
}

func TestSigningPayloadHasContextPrefix( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)

	payload, err := signingPayload(cert)
	if err != nil {
		t.Fatalf("signingPayload: %v", err)
	}

	prefix := string(
		payload[:len(ctxNodeAdmissionV1)],
	)
	if prefix != ctxNodeAdmissionV1 {
		t.Errorf(
			"prefix = %q, want %q",
			prefix, ctxNodeAdmissionV1,
		)
	}
}

func TestSigningPayloadContainsCanonical( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeUser,
	)

	payload, _ := signingPayload(cert)
	canon, _ := canonicalSerialize(cert)

	tail := payload[len(ctxNodeAdmissionV1):]
	if !bytes.Equal(tail, canon) {
		t.Error(
			"payload tail should equal " +
				"canonical serialization",
		)
	}
}

func TestPopSigningPayloadPrefix( // A
	t *testing.T,
) {
	t.Parallel()

	challenge := []byte("test-challenge-data")
	payload := popSigningPayload(challenge)

	prefix := string(
		payload[:len(ctxNodePopV1)],
	)
	if prefix != ctxNodePopV1 {
		t.Errorf(
			"prefix = %q, want %q",
			prefix, ctxNodePopV1,
		)
	}
}

func TestPopSigningPayloadContainsChallenge( // A
	t *testing.T,
) {
	t.Parallel()

	challenge := []byte("my-challenge-bytes")
	payload := popSigningPayload(challenge)

	tail := payload[len(ctxNodePopV1):]
	if !bytes.Equal(tail, challenge) {
		t.Error(
			"payload tail should equal challenge",
		)
	}
}

func TestPopSigningPayloadDeterministic( // A
	t *testing.T,
) {
	t.Parallel()

	challenge := []byte("deterministic-test")

	p1 := popSigningPayload(challenge)
	p2 := popSigningPayload(challenge)

	if !bytes.Equal(p1, p2) {
		t.Error(
			"popSigningPayload not deterministic",
		)
	}
}

func TestPopSigningPayloadDiffChallenge( // A
	t *testing.T,
) {
	t.Parallel()

	p1 := popSigningPayload([]byte("alpha"))
	p2 := popSigningPayload([]byte("beta"))

	if bytes.Equal(p1, p2) {
		t.Error(
			"different challenges should produce " +
				"different payloads",
		)
	}
}

func TestPopSigningPayloadEmpty( // A
	t *testing.T,
) {
	t.Parallel()

	payload := popSigningPayload(nil)
	if !bytes.Equal(
		payload, []byte(ctxNodePopV1),
	) {
		t.Error(
			"nil challenge should yield only prefix",
		)
	}
}

func TestDelegationSigningPayloadPrefix( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	sessionAC := generateKeys(t)
	sessionPub := pubKeyPtr(t, sessionAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	x509FP := testNonce(t)
	proof := buildTestDelegation(
		t, sessionPub, cert, x509FP,
	)

	payload, err := delegationSigningPayload(proof)
	if err != nil {
		t.Fatalf(
			"delegationSigningPayload: %v", err,
		)
	}

	prefix := string(
		payload[:len(ctxNodeDelegationV1)],
	)
	if prefix != ctxNodeDelegationV1 {
		t.Errorf(
			"prefix = %q, want %q",
			prefix, ctxNodeDelegationV1,
		)
	}
}

func TestDelegationSigningPayloadContainsCanon( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	sessionAC := generateKeys(t)
	sessionPub := pubKeyPtr(t, sessionAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeUser,
	)
	x509FP := testNonce(t)
	proof := buildTestDelegation(
		t, sessionPub, cert, x509FP,
	)

	payload, _ := delegationSigningPayload(proof)
	canon, _ := canonicalSerializeDelegation(proof)

	tail := payload[len(ctxNodeDelegationV1):]
	if !bytes.Equal(tail, canon) {
		t.Error(
			"delegation payload tail should " +
				"equal canonical serialization",
		)
	}
}

func TestDelegationSigningPayloadNilProof( // A
	t *testing.T,
) {
	t.Parallel()

	_, err := delegationSigningPayload(nil)
	if err == nil {
		t.Fatal("expected error for nil proof")
	}
	if !strings.Contains(
		err.Error(), "must not be nil",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDelegationSigningPayloadDeterministic( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	sessionAC := generateKeys(t)
	sessionPub := pubKeyPtr(t, sessionAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	x509FP := testNonce(t)
	proof := buildTestDelegation(
		t, sessionPub, cert, x509FP,
	)

	p1, _ := delegationSigningPayload(proof)
	p2, _ := delegationSigningPayload(proof)

	if !bytes.Equal(p1, p2) {
		t.Error(
			"delegationSigningPayload " +
				"not deterministic",
		)
	}
}

func TestComputeCAHashDeterministic( // A
	t *testing.T,
) {
	t.Parallel()

	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)

	h1, err := computeCAHash(pub)
	if err != nil {
		t.Fatalf("computeCAHash 1: %v", err)
	}

	h2, err := computeCAHash(pub)
	if err != nil {
		t.Fatalf("computeCAHash 2: %v", err)
	}

	if !h1.Equal(h2) {
		t.Error("computeCAHash not deterministic")
	}
}

func TestComputeCAHashDifferentKeys( // A
	t *testing.T,
) {
	t.Parallel()

	ac1 := generateKeys(t)
	ac2 := generateKeys(t)

	h1, _ := computeCAHash(pubKeyPtr(t, ac1))
	h2, _ := computeCAHash(pubKeyPtr(t, ac2))

	if h1.Equal(h2) {
		t.Error(
			"different keys should produce " +
				"different CA hashes",
		)
	}
}

func TestComputeCAHashNotZero(t *testing.T) { // A
	t.Parallel()

	ac := generateKeys(t)
	h, err := computeCAHash(pubKeyPtr(t, ac))
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}
	if h.IsZero() {
		t.Error("CA hash should not be zero")
	}
}

func TestVerifyNodeCertValid(t *testing.T) { // A
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	sig := signCert(t, caAC, cert)

	nodeID, err := verifyNodeCert(
		caPub, cert, sig,
	)
	if err != nil {
		t.Fatalf("verifyNodeCert: %v", err)
	}

	expected, _ := nodePub.NodeID()
	if nodeID != expected {
		t.Errorf(
			"nodeID = %v, want %v",
			nodeID, expected,
		)
	}
}

func TestVerifyNodeCertUserRole(t *testing.T) { // A
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeUser,
	)
	sig := signCert(t, caAC, cert)

	_, err := verifyNodeCert(caPub, cert, sig)
	if err != nil {
		t.Fatalf(
			"verifyNodeCert with ScopeUser: %v",
			err,
		)
	}
}

func TestVerifyNodeCertWrongSignature( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)

	badSig := []byte("not-a-valid-signature")
	_, err := verifyNodeCert(
		caPub, cert, badSig,
	)
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
	if !strings.Contains(
		err.Error(), "verification failed",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyNodeCertWrongCA(t *testing.T) { // A
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	sig := signCert(t, caAC, cert)

	_, err := verifyNodeCert(
		otherPub, cert, sig,
	)
	if err == nil {
		t.Fatal("expected error for wrong CA key")
	}
}

func TestVerifyNodeCertNilCA(t *testing.T) { // A
	t.Parallel()

	nodeAC := generateKeys(t)
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	sig := signCert(t, caAC, cert)

	_, err := verifyNodeCert(nil, cert, sig)
	if err == nil {
		t.Fatal("expected error for nil CA key")
	}
	if !strings.Contains(
		err.Error(), "CA public key",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyNodeCertNilCert( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)

	_, err := verifyNodeCert(
		caPub, nil, []byte("sig"),
	)
	if err == nil {
		t.Fatal("expected error for nil cert")
	}
	if !strings.Contains(
		err.Error(), "node cert must not be nil",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyNodeCertNilNodePubKey( // A
	t *testing.T,
) {
	t.Parallel()

	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	caHash, err := computeCAHash(caPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	// NewNodeCert rejects nil public key; verify that.
	_, err = NewNodeCert(NodeCertParams{
		NodePubKey:   nil,
		IssuerCAHash: caHash,
		Serial:       testSerial(t),
		CertNonce:    testNonce(t),
	})
	if err == nil {
		t.Fatal(
			"NewNodeCert should reject nil public key",
		)
	}
	if !strings.Contains(
		err.Error(), "node public key",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}
