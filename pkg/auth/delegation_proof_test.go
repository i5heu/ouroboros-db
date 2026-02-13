package auth

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func validDelegationProofParams( // A
	t *testing.T,
) DelegationProofParams {
	t.Helper()
	now := time.Now().UTC()
	return DelegationProofParams{
		TLSCertPubKeyHash: testNonce(t),
		TLSExporterBinding: testNonce(t),
		X509Fingerprint: testNonce(t),
		NodeCertHash:    HashBytes([]byte("node-cert")),
		NotBefore:       now.Add(-time.Minute),
		NotAfter:        now.Add(time.Minute),
		HandshakeNonce:  testNonce(t),
	}
}

func TestNewDelegationProofValidation( // A
	t *testing.T,
) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*DelegationProofParams)
		want   string
	}{
		{
			name: "zero TLS cert pubkey hash",
			mutate: func(p *DelegationProofParams) {
				p.TLSCertPubKeyHash = [32]byte{}
			},
			want: "TLS cert pubkey hash",
		},
		{
			name: "zero TLS exporter binding",
			mutate: func(p *DelegationProofParams) {
				p.TLSExporterBinding = [32]byte{}
			},
			want: "TLS exporter binding",
		},
		{
			name: "invalid time ordering",
			mutate: func(p *DelegationProofParams) {
				now := time.Now().UTC()
				p.NotBefore = now
				p.NotAfter = now
			},
			want: "notAfter",
		},
		{
			name: "zero x509 fingerprint",
			mutate: func(p *DelegationProofParams) {
				p.X509Fingerprint = [32]byte{}
			},
			want: "x509 fingerprint",
		},
		{
			name: "zero node cert hash",
			mutate: func(p *DelegationProofParams) {
				p.NodeCertHash = CaHash{}
			},
			want: "node cert hash",
		},
		{
			name: "zero handshake nonce",
			mutate: func(p *DelegationProofParams) {
				p.HandshakeNonce = [32]byte{}
			},
			want: "handshake nonce",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { // A
			params := validDelegationProofParams(t)
			tc.mutate(&params)
			_, err := NewDelegationProof(params)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want contains %q", err, tc.want)
			}
		})
	}
}

func TestDelegationProofAccessors( // A
	t *testing.T,
) {
	t.Parallel()
	params := validDelegationProofParams(t)
	proof, err := NewDelegationProof(params)
	if err != nil {
		t.Fatalf("NewDelegationProof: %v", err)
	}

	if proof.TLSCertPubKeyHash() != params.TLSCertPubKeyHash {
		t.Fatal("TLS cert pubkey hash mismatch")
	}
	if proof.TLSExporterBinding() != params.TLSExporterBinding {
		t.Fatal("TLS exporter binding mismatch")
	}
	if proof.X509Fingerprint() != params.X509Fingerprint {
		t.Fatal("x509 fingerprint mismatch")
	}
	if !proof.NodeCertHash().Equal(params.NodeCertHash) {
		t.Fatal("node cert hash mismatch")
	}
	if !proof.NotBefore().Equal(params.NotBefore.UTC()) {
		t.Fatal("notBefore mismatch")
	}
	if !proof.NotAfter().Equal(params.NotAfter.UTC()) {
		t.Fatal("notAfter mismatch")
	}
	if proof.HandshakeNonce() != params.HandshakeNonce {
		t.Fatal("handshake nonce mismatch")
	}
}

func TestCanonicalSerializeDelegationDeterministic( // A
	t *testing.T,
) {
	t.Parallel()
	params := validDelegationProofParams(t)
	proof, err := NewDelegationProof(params)
	if err != nil {
		t.Fatalf("NewDelegationProof: %v", err)
	}

	b1, err := canonicalSerializeDelegation(proof)
	if err != nil {
		t.Fatalf("canonicalSerializeDelegation(1): %v", err)
	}
	b2, err := canonicalSerializeDelegation(proof)
	if err != nil {
		t.Fatalf("canonicalSerializeDelegation(2): %v", err)
	}

	if !bytes.Equal(b1, b2) {
		t.Fatal("delegation canonical serialization is not deterministic")
	}
}

func TestDelegationProofWireRoundTrip( // A
	t *testing.T,
) {
	t.Parallel()
	params := validDelegationProofParams(t)
	proof, err := NewDelegationProof(params)
	if err != nil {
		t.Fatalf("NewDelegationProof: %v", err)
	}

	wire, err := MarshalDelegationProof(proof)
	if err != nil {
		t.Fatalf("MarshalDelegationProof: %v", err)
	}

	roundTrip, err := UnmarshalDelegationProof(wire)
	if err != nil {
		t.Fatalf("UnmarshalDelegationProof: %v", err)
	}

	if roundTrip.TLSCertPubKeyHash() != proof.TLSCertPubKeyHash() {
		t.Fatal("TLS cert pubkey hash changed in round trip")
	}
	if roundTrip.TLSExporterBinding() != proof.TLSExporterBinding() {
		t.Fatal("TLS exporter binding changed in round trip")
	}
	if roundTrip.X509Fingerprint() != proof.X509Fingerprint() {
		t.Fatal("x509 fingerprint changed in round trip")
	}
	if !roundTrip.NodeCertHash().Equal(proof.NodeCertHash()) {
		t.Fatal("node cert hash changed in round trip")
	}
	if roundTrip.NotBefore().Unix() != proof.NotBefore().Unix() {
		t.Fatal("notBefore changed in round trip")
	}
	if roundTrip.NotAfter().Unix() != proof.NotAfter().Unix() {
		t.Fatal("notAfter changed in round trip")
	}
	if roundTrip.HandshakeNonce() != proof.HandshakeNonce() {
		t.Fatal("handshake nonce changed in round trip")
	}
}

func TestUnmarshalDelegationProofRejectsMalformed( // A
	t *testing.T,
) {
	t.Parallel()

	t.Run("too short", func(t *testing.T) { // A
		_, err := UnmarshalDelegationProof([]byte{1, 2, 3})
		if err == nil {
			t.Fatal("expected error for short payload")
		}
	})

	t.Run("trailing bytes", func(t *testing.T) { // A
		params := validDelegationProofParams(t)
		proof, err := NewDelegationProof(params)
		if err != nil {
			t.Fatalf("NewDelegationProof: %v", err)
		}
		wire, err := MarshalDelegationProof(proof)
		if err != nil {
			t.Fatalf("MarshalDelegationProof: %v", err)
		}
		wire = append(wire, 0xFF)
		_, err = UnmarshalDelegationProof(wire)
		if err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Fatalf("error = %v, want trailing bytes error", err)
		}
	})

	t.Run("invalid sized field", func(t *testing.T) { // A
		_, err := UnmarshalDelegationProof(
			[]byte{0, 0, 0, 5, 1, 2, 3},
		)
		if err == nil {
			t.Fatal("expected error for invalid field length")
		}
	})
}
