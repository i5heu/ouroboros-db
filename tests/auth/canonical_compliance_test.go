package auth_test // A

import (
	"bytes"
	"testing"

	"github.com/i5heu/ouroboros-db/pkg/auth"
	"pgregory.net/rapid"
)

func TestPropertyCanonicalNodeCertDeterministic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		enc1, err := auth.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("first encoding: %v", err)
		}
		enc2, err := auth.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("second encoding: %v", err)
		}

		if !bytes.Equal(enc1, enc2) {
			rt.Fatalf(
				"non-deterministic encoding: %x != %x",
				enc1, enc2,
			)
		}
	})
}

func TestPropertyCanonicalNodeCertNotEmpty(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		enc, err := auth.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("encoding: %v", err)
		}

		if len(enc) == 0 {
			rt.Fatal("encoding is empty")
		}
	})
}

func TestPropertyCanonicalBundleDeterministic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)

		cd1 := genCertData(kp, now).Draw(rt, "certData1")
		cd2 := genCertData(kp, now).Draw(rt, "certData2")
		cd2.serial = []byte("different-serial")

		cert1, err := cd1.toNodeCert()
		if err != nil {
			rt.Fatalf("cert1: %v", err)
		}
		cert2, err := cd2.toNodeCert()
		if err != nil {
			rt.Fatalf("cert2: %v", err)
		}

		certs := []auth.NodeCertLike{cert1, cert2}
		enc1, err := auth.CanonicalNodeCertBundle(certs)
		if err != nil {
			rt.Fatalf("first bundle: %v", err)
		}
		enc2, err := auth.CanonicalNodeCertBundle(certs)
		if err != nil {
			rt.Fatalf("second bundle: %v", err)
		}

		if !bytes.Equal(enc1, enc2) {
			rt.Fatalf(
				"non-deterministic bundle: %x != %x",
				enc1, enc2,
			)
		}
	})
}

func TestPropertyCanonicalBundleOrderIndependent(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)

		cd1 := genCertData(kp, now).Draw(rt, "certData1")
		cd2 := genCertData(kp, now).Draw(rt, "certData2")
		cd2.serial = []byte("serial-b")

		cert1, err := cd1.toNodeCert()
		if err != nil {
			rt.Fatalf("cert1: %v", err)
		}
		cert2, err := cd2.toNodeCert()
		if err != nil {
			rt.Fatalf("cert2: %v", err)
		}

		order1 := []auth.NodeCertLike{cert1, cert2}
		order2 := []auth.NodeCertLike{cert2, cert1}

		enc1, err := auth.CanonicalNodeCertBundle(order1)
		if err != nil {
			rt.Fatalf("order1 bundle: %v", err)
		}
		enc2, err := auth.CanonicalNodeCertBundle(order2)
		if err != nil {
			rt.Fatalf("order2 bundle: %v", err)
		}

		if !bytes.Equal(enc1, enc2) {
			rt.Fatalf(
				"bundle not order-independent:\n%x\n!=\n%x",
				enc1, enc2,
			)
		}
	})
}

func TestPropertyCanonicalBundleSingleCert(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		bundle, err := auth.CanonicalNodeCertBundle(
			[]auth.NodeCertLike{cert},
		)
		if err != nil {
			rt.Fatalf("bundle: %v", err)
		}

		if len(bundle) == 0 {
			rt.Fatal("single-cert bundle is empty")
		}
	})
}

func TestPropertyCanonicalDelegationDeterministic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := int64(1000000)
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		enc1, err := auth.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("first encoding: %v", err)
		}
		enc2, err := auth.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("second encoding: %v", err)
		}

		if !bytes.Equal(enc1, enc2) {
			rt.Fatalf(
				"non-deterministic delegation: %x != %x",
				enc1, enc2,
			)
		}
	})
}

func TestPropertyCanonicalDelegationForExporterDiffers(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		tls := genTLSSession().Draw(rt, "tls")
		now := int64(1000000)
		dd := genDelegationData(tls, now).Draw(rt, "delData")
		proof := dd.toDelegationProof()

		full, err := auth.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("full encoding: %v", err)
		}
		noExp, err := auth.CanonicalDelegationProofForExporter(proof)
		if err != nil {
			rt.Fatalf("exporter encoding: %v", err)
		}

		if bytes.Equal(full, noExp) {
			rt.Fatal(
				"exporter variant should differ from full",
			)
		}
	})
}

func TestPropertyDomainSeparationPrepends(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ctx := rapid.StringOfN(
			rapid.RuneFrom([]rune("abcdefghijklmnopqrstuvwxyz")),
			1, 32, 32,
		).Draw(rt, "ctx")
		data := rapid.SliceOfN(
			rapid.Byte(), 0, 64,
		).Draw(rt, "data")

		result := auth.DomainSeparate(ctx, data)
		expected := append([]byte(ctx), data...)

		if !bytes.Equal(result, expected) {
			rt.Fatalf(
				"DomainSeparate(%q, %x) = %x, want %x",
				ctx, data, result, expected,
			)
		}
	})
}

func TestPropertyDomainSeparationLength(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ctx := rapid.StringOfN(
			rapid.RuneFrom([]rune("abcdefghijklmnopqrstuvwxyz")),
			1, 32, 32,
		).Draw(rt, "ctx")
		data := rapid.SliceOfN(
			rapid.Byte(), 0, 64,
		).Draw(rt, "data")

		result := auth.DomainSeparate(ctx, data)
		expectedLen := len(ctx) + len(data)

		if len(result) != expectedLen {
			rt.Fatalf(
				"DomainSeparate length = %d, want %d",
				len(result), expectedLen,
			)
		}
	})
}
