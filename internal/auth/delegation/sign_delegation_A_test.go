package delegation_test

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/internal/auth/delegation"
)

func TestSignDelegation(t *testing.T) { // A
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("NewAsyncCrypt: %v", err)
	}

	si, err := delegation.NewSessionIdentity(5 * time.Minute)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	pub := ac.GetPublicKey()
	cert, err := certpkg.NewNodeCert(
		pub,
		"ca-hash-test",
		time.Now().Unix()-60,
		time.Now().Unix()+3600,
		[]byte("serial-1"),
		[]byte("nonce-1"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	certs := []canonical.NodeCertLike{cert}

	fakeExporter := func(
		label string,
		ctx []byte,
		length int,
	) ([]byte, error) {
		out := make([]byte, length)
		switch label {
		case delegation.TranscriptBindingLabel:
			for i := range out {
				out[i] = byte(i)
			}
		case delegation.ExporterLabel:
			for i := range out {
				out[i] = 0xAB
			}
		default:
			t.Fatalf(
				"unexpected label: %s", label,
			)
		}
		return out, nil
	}

	proof, sig, err := delegation.SignDelegation(
		ac, certs, si, fakeExporter,
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	if len(proof.TLSCertPubKeyHash()) != 32 {
		t.Fatal("TLSCertPubKeyHash wrong size")
	}
	if len(proof.TLSExporterBinding()) != 32 {
		t.Fatal("TLSExporterBinding wrong size")
	}
	if len(proof.TLSTranscriptHash()) != 32 {
		t.Fatal("TLSTranscriptHash wrong size")
	}
	if len(proof.X509Fingerprint()) != 32 {
		t.Fatal("X509Fingerprint wrong size")
	}
	if len(proof.NodeCertBundleHash()) != 32 {
		t.Fatal("NodeCertBundleHash wrong size")
	}
	if proof.NotAfter()-proof.NotBefore() != delegation.MaxDelegationTTL {
		t.Fatal("TTL not MaxDelegationTTL")
	}

	if !bytes.Equal(
		proof.TLSCertPubKeyHash(),
		si.CertPubKeyHash[:],
	) {
		t.Fatal("CertPubKeyHash mismatch")
	}

	if !bytes.Equal(
		proof.X509Fingerprint(),
		si.X509Fingerprint[:],
	) {
		t.Fatal("X509Fingerprint mismatch")
	}

	canon, err := canonical.CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatalf("canonical proof: %v", err)
	}
	msg := canonical.DomainSeparate(delegation.CTXNodeDelegationV1, canon)
	if !pub.Verify(msg, sig) {
		t.Fatal(
			"delegation signature failed verification",
		)
	}
}

func TestSignDelegationBundleHash(t *testing.T) { // A
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("NewAsyncCrypt: %v", err)
	}
	si, err := delegation.NewSessionIdentity(5 * time.Minute)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}
	pub := ac.GetPublicKey()
	cert, err := certpkg.NewNodeCert(
		pub, "ca-hash",
		time.Now().Unix()-60,
		time.Now().Unix()+3600,
		[]byte("serial"), []byte("nonce"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	certs := []canonical.NodeCertLike{cert}

	proof, _, err := delegation.SignDelegation(
		ac, certs, si,
		func(
			_ string, _ []byte, l int,
		) ([]byte, error) {
			return make([]byte, l), nil
		},
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	bundleBytes, err := canonical.CanonicalNodeCertBundle(certs)
	if err != nil {
		t.Fatalf("CanonicalNodeCertBundle: %v", err)
	}
	want := sha256.Sum256(bundleBytes)
	if !bytes.Equal(
		proof.NodeCertBundleHash(), want[:],
	) {
		t.Fatal("bundle hash mismatch")
	}
}
