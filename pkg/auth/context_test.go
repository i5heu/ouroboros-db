package auth

import (
	"bytes"
	"strings"
	"testing"
)

func TestCanonicalSerializeDeterministic( // A
	t *testing.T,
) {
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(pub, caHash)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

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
	ac1 := generateKeys(t)
	ac2 := generateKeys(t)
	caHash := HashBytes([]byte("test-ca"))

	cert1, _ := NewNodeCert(
		pubKeyPtr(t, ac1), caHash,
	)
	cert2, _ := NewNodeCert(
		pubKeyPtr(t, ac2), caHash,
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
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("ca"))

	cert, _ := NewNodeCert(pub, caHash)
	b, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize: %v", err)
	}

	if b[0] != certVersion {
		t.Errorf(
			"first byte = %d, want %d",
			b[0], certVersion,
		)
	}
}

func TestCanonicalSerializeEndsWithCAHash( // A
	t *testing.T,
) {
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("ca"))

	cert, _ := NewNodeCert(pub, caHash)
	b, err := canonicalSerialize(cert)
	if err != nil {
		t.Fatalf("canonicalSerialize: %v", err)
	}

	suffix := b[len(b)-32:]
	if !bytes.Equal(suffix, caHash.Bytes()) {
		t.Error(
			"serialization should end with " +
				"IssuerCAHash",
		)
	}
}

func TestSigningPayloadHasContextPrefix( // A
	t *testing.T,
) {
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("ca"))

	cert, _ := NewNodeCert(pub, caHash)
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
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("ca"))

	cert, _ := NewNodeCert(pub, caHash)
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

func TestComputeCAHashDeterministic( // A
	t *testing.T,
) {
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
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(t, caPub, nodePub)
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

func TestVerifyNodeCertWrongSignature( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(t, caPub, nodePub)

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
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, err := verifyNodeCert(
		otherPub, cert, sig,
	)
	if err == nil {
		t.Fatal("expected error for wrong CA key")
	}
}
