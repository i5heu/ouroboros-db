package auth_test // A

import (
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db/internal/auth/delegation"
	"pgregory.net/rapid"
)

func TestPropertyDelegationTTLBounded(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := time.Now().Unix()
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		ttl := proof.NotAfter() - proof.NotBefore()
		if ttl > delegation.MaxDelegationTTL {
			rt.Fatalf(
				"TTL %d exceeds MaxDelegationTTL %d",
				ttl, delegation.MaxDelegationTTL,
			)
		}
		if ttl <= 0 {
			rt.Fatalf("TTL must be positive, got %d", ttl)
		}
	})
}

func TestPropertyDelegationTimeWindow(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := time.Now().Unix()
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		if proof.NotAfter() <= proof.NotBefore() {
			rt.Fatalf(
				"NotAfter (%d) must be > NotBefore (%d)",
				proof.NotAfter(), proof.NotBefore(),
			)
		}
	})
}

func TestPropertyDelegationAllFieldsSet(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := time.Now().Unix()
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		if len(proof.TLSCertPubKeyHash()) == 0 {
			rt.Fatal("TLSCertPubKeyHash is empty")
		}
		if len(proof.TLSExporterBinding()) == 0 {
			rt.Fatal("TLSExporterBinding is empty")
		}
		if len(proof.TLSTranscriptHash()) == 0 {
			rt.Fatal("TLSTranscriptHash is empty")
		}
		if len(proof.X509Fingerprint()) == 0 {
			rt.Fatal("X509Fingerprint is empty")
		}
		if len(proof.NodeCertBundleHash()) == 0 {
			rt.Fatal("NodeCertBundleHash is empty")
		}
	})
}

func TestPropertyDelegationFieldsMatchInput(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := time.Now().Unix()
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		if string(proof.TLSCertPubKeyHash()) != string(tls.certPubKeyHash) {
			rt.Fatal("TLSCertPubKeyHash mismatch")
		}
		if string(proof.TLSExporterBinding()) != string(tls.exporterBinding) {
			rt.Fatal("TLSExporterBinding mismatch")
		}
		if string(proof.TLSTranscriptHash()) != string(tls.transcriptHash) {
			rt.Fatal("TLSTranscriptHash mismatch")
		}
		if string(proof.X509Fingerprint()) != string(tls.x509Fingerprint) {
			rt.Fatal("X509Fingerprint mismatch")
		}
		if string(proof.NodeCertBundleHash()) != string(dd.bundleHash) {
			rt.Fatal("NodeCertBundleHash mismatch")
		}
		if proof.NotBefore() != dd.notBefore {
			rt.Fatal("NotBefore mismatch")
		}
		if proof.NotAfter() != dd.notAfter {
			rt.Fatal("NotAfter mismatch")
		}
	})
}

func TestPropertyDelegationShortTTLRequired(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")

		now := time.Now().Unix()
		ttl := rapid.Int64Range(10, delegation.MaxDelegationTTL).Draw(rt, "ttl")
		notBefore := now - 5
		notAfter := notBefore + ttl

		proof := delegation.NewDelegationProof(
			tls.certPubKeyHash,
			tls.exporterBinding,
			tls.transcriptHash,
			tls.x509Fingerprint,
			[]byte("bundle-hash"),
			notBefore,
			notAfter,
		)

		actualTTL := proof.NotAfter() - proof.NotBefore()
		if actualTTL != ttl {
			rt.Fatalf("TTL mismatch: got %d, want %d", actualTTL, ttl)
		}
		if actualTTL > delegation.MaxDelegationTTL {
			rt.Fatalf(
				"TTL %d exceeds MaxDelegationTTL %d",
				actualTTL, delegation.MaxDelegationTTL,
			)
		}
	})
}

func TestPropertyExporterContextOmitsBinding(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := time.Now().Unix()
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		full, err := canonical.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("full: %v", err)
		}
		noExp, err := canonical.CanonicalDelegationProofForExporter(proof)
		if err != nil {
			rt.Fatalf("noExp: %v", err)
		}

		if len(full) <= len(noExp) {
			rt.Fatalf(
				"full encoding should be larger than exporter-only: "+
					"full=%d, noExp=%d",
				len(full), len(noExp),
			)
		}
	})
}
