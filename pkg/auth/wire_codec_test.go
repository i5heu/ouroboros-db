package auth

import (
	"strings"
	"testing"
	"time"
)

func TestMarshalNodeCertNil(t *testing.T) { // A
	t.Parallel()
	_, err := MarshalNodeCert(nil)
	if err == nil || !strings.Contains(err.Error(), "must not be nil") {
		t.Fatalf("err = %v, want nil error", err)
	}
}

func TestUnmarshalNodeCertRoundTrip(t *testing.T) { // A
	t.Parallel()
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	cert := buildTestCert(t, caPub, nodePub, ScopeAdmin)

	wire, err := MarshalNodeCert(cert)
	if err != nil {
		t.Fatalf("MarshalNodeCert: %v", err)
	}

	roundTrip, err := UnmarshalNodeCert(wire)
	if err != nil {
		t.Fatalf("UnmarshalNodeCert: %v", err)
	}

	if roundTrip.CertVersion() != cert.CertVersion() {
		t.Fatal("cert version mismatch")
	}
	if !roundTrip.IssuerCAHash().Equal(cert.IssuerCAHash()) {
		t.Fatal("issuer mismatch")
	}
	if roundTrip.RoleClaims() != cert.RoleClaims() {
		t.Fatal("role mismatch")
	}
}

func TestUnmarshalNodeCertRejectsMalformed(t *testing.T) { // A
	t.Parallel()
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	cert := buildTestCert(t, caPub, nodePub, ScopeAdmin)
	wire, err := MarshalNodeCert(cert)
	if err != nil {
		t.Fatalf("MarshalNodeCert: %v", err)
	}

	t.Run("too short", func(t *testing.T) { // A
		_, err := UnmarshalNodeCert([]byte{0x01, 0x02})
		if err == nil {
			t.Fatal("expected short payload error")
		}
	})

	t.Run("bad version", func(t *testing.T) { // A
		bad := append([]byte{}, wire...)
		bad[0] = 0xFF
		_, err := UnmarshalNodeCert(bad)
		if err == nil || !strings.Contains(err.Error(), "unsupported") {
			t.Fatalf("err = %v, want unsupported version", err)
		}
	})

	t.Run("trailing bytes", func(t *testing.T) { // A
		bad := append(append([]byte{}, wire...), 0x01)
		_, err := UnmarshalNodeCert(bad)
		if err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Fatalf("err = %v, want trailing bytes", err)
		}
	})
}

func TestReadSizedFieldValidation(t *testing.T) { // A
	t.Parallel()
	_, _, err := readSizedField([]byte{0x01}, 0)
	if err == nil || !strings.Contains(err.Error(), "missing length") {
		t.Fatalf("err = %v, want missing length", err)
	}

	_, _, err = readSizedField([]byte{0, 0, 0, 5, 1}, 0)
	if err == nil || !strings.Contains(err.Error(), "invalid field length") {
		t.Fatalf("err = %v, want invalid field length", err)
	}
}

func TestUnmarshalDelegationProofRejectsMalformedLengths( // A
	t *testing.T,
) {
	t.Parallel()
	_, err := UnmarshalDelegationProof([]byte{0x01, 0x02, 0x03})
	if err == nil {
		t.Fatal("expected short payload error")
	}

	now := time.Now().UTC()
	proof, err := NewDelegationProof(DelegationProofParams{
		TLSCertPubKeyHash: testNonce(t),
		TLSExporterBinding: testNonce(t),
		X509Fingerprint: testNonce(t),
		NodeCertHash: HashBytes([]byte("cert")),
		NotBefore: now.Add(-time.Minute),
		NotAfter: now.Add(time.Minute),
		HandshakeNonce: testNonce(t),
	})
	if err != nil {
		t.Fatalf("NewDelegationProof: %v", err)
	}
	wire, err := MarshalDelegationProof(proof)
	if err != nil {
		t.Fatalf("MarshalDelegationProof: %v", err)
	}
	wire = append(wire, 0x01)
	_, err = UnmarshalDelegationProof(wire)
	if err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("err = %v, want trailing bytes", err)
	}
}
