package auth

import "testing"

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
	var _ DelegationProofLike = proof
}
