package delegation

import (
	"bytes"
	"testing"

	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
)

func TestNewDelegationProof(t *testing.T) { // A
	proof := NewDelegationProof(
		[]byte("cert-hash"),
		[]byte("exporter"),
		[]byte("transcript"),
		[]byte("x509"),
		[]byte("bundle"),
		100, 400,
	)

	if string(proof.TLSCertPubKeyHash()) != "cert-hash" {
		t.Error("wrong TLSCertPubKeyHash")
	}
	if string(proof.TLSExporterBinding()) != "exporter" {
		t.Error("wrong TLSExporterBinding")
	}
	if string(proof.TLSTranscriptHash()) != "transcript" {
		t.Error("wrong TLSTranscriptHash")
	}
	if string(proof.X509Fingerprint()) != "x509" {
		t.Error("wrong X509Fingerprint")
	}
	if string(proof.NodeCertBundleHash()) != "bundle" {
		t.Error("wrong NodeCertBundleHash")
	}
	if proof.NotBefore() != 100 {
		t.Error("wrong NotBefore")
	}
	if proof.NotAfter() != 400 {
		t.Error("wrong NotAfter")
	}
}

func TestDelegationProofInterface(t *testing.T) { // A
	proof := NewDelegationProof(
		nil, nil, nil, nil, nil, 0, 0,
	)
	var _ canonical.DelegationProofLike = proof
}

func assertConstructorCopies( // A
	t *testing.T,
	proof canonical.DelegationProofLike,
) {
	t.Helper()
	if !bytes.Equal(proof.TLSCertPubKeyHash(), []byte("cert-hash")) {
		t.Fatalf("TLSCertPubKeyHash mutated after constructor copy")
	}
	if !bytes.Equal(proof.TLSExporterBinding(), []byte("exporter")) {
		t.Fatalf("TLSExporterBinding mutated after constructor copy")
	}
	if !bytes.Equal(proof.TLSTranscriptHash(), []byte("transcript")) {
		t.Fatalf("TLSTranscriptHash mutated after constructor copy")
	}
	if !bytes.Equal(proof.X509Fingerprint(), []byte("x509")) {
		t.Fatalf("X509Fingerprint mutated after constructor copy")
	}
	if !bytes.Equal(proof.NodeCertBundleHash(), []byte("bundle")) {
		t.Fatalf("NodeCertBundleHash mutated after constructor copy")
	}
}

func assertGetterCopies( // A
	t *testing.T,
	proof canonical.DelegationProofLike,
) {
	t.Helper()
	returnedCertHash := proof.TLSCertPubKeyHash()
	returnedExporter := proof.TLSExporterBinding()
	returnedTranscript := proof.TLSTranscriptHash()
	returnedX509 := proof.X509Fingerprint()
	returnedBundle := proof.NodeCertBundleHash()

	returnedCertHash[0] = 'A'
	returnedExporter[0] = 'B'
	returnedTranscript[0] = 'C'
	returnedX509[0] = 'D'
	returnedBundle[0] = 'E'

	if !bytes.Equal(proof.TLSCertPubKeyHash(), []byte("cert-hash")) {
		t.Fatalf("TLSCertPubKeyHash mutated through getter")
	}
	if !bytes.Equal(proof.TLSExporterBinding(), []byte("exporter")) {
		t.Fatalf("TLSExporterBinding mutated through getter")
	}
	if !bytes.Equal(proof.TLSTranscriptHash(), []byte("transcript")) {
		t.Fatalf("TLSTranscriptHash mutated through getter")
	}
	if !bytes.Equal(proof.X509Fingerprint(), []byte("x509")) {
		t.Fatalf("X509Fingerprint mutated through getter")
	}
	if !bytes.Equal(proof.NodeCertBundleHash(), []byte("bundle")) {
		t.Fatalf("NodeCertBundleHash mutated through getter")
	}
}

func TestDelegationProofDefensiveCopiesByteFields( // A
	t *testing.T,
) {
	tlsCertHash := []byte("cert-hash")
	exporter := []byte("exporter")
	transcript := []byte("transcript")
	x509 := []byte("x509")
	bundle := []byte("bundle")

	proof := NewDelegationProof(
		tlsCertHash,
		exporter,
		transcript,
		x509,
		bundle,
		100,
		400,
	)

	tlsCertHash[0] = 'X'
	exporter[0] = 'Y'
	transcriptHash := transcript
	transcriptHash[0] = 'Z'
	x509[0] = 'W'
	bundle[0] = 'Q'

	assertConstructorCopies(t, proof)
	assertGetterCopies(t, proof)
}
