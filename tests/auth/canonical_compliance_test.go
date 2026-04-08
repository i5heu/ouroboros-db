package auth_test // A

import (
	"bytes"
	"testing"

	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
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

		enc1, err := canonical.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("first encoding: %v", err)
		}
		enc2, err := canonical.CanonicalNodeCert(cert)
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

		enc, err := canonical.CanonicalNodeCert(cert)
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

		certs := []canonical.NodeCertLike{cert1, cert2}
		enc1, err := canonical.CanonicalNodeCertBundle(certs)
		if err != nil {
			rt.Fatalf("first bundle: %v", err)
		}
		enc2, err := canonical.CanonicalNodeCertBundle(certs)
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

		order1 := []canonical.NodeCertLike{cert1, cert2}
		order2 := []canonical.NodeCertLike{cert2, cert1}

		enc1, err := canonical.CanonicalNodeCertBundle(order1)
		if err != nil {
			rt.Fatalf("order1 bundle: %v", err)
		}
		enc2, err := canonical.CanonicalNodeCertBundle(order2)
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

		bundle, err := canonical.CanonicalNodeCertBundle(
			[]canonical.NodeCertLike{cert},
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

		enc1, err := canonical.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("first encoding: %v", err)
		}
		enc2, err := canonical.CanonicalDelegationProof(proof)
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

		full, err := canonical.CanonicalDelegationProof(proof)
		if err != nil {
			rt.Fatalf("full encoding: %v", err)
		}
		noExp, err := canonical.CanonicalDelegationProofForExporter(proof)
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

		result := canonical.DomainSeparate(ctx, data)

		// Expect 4-byte big-endian length prefix.
		prefix := []byte(ctx)
		l := len(prefix)
		expected := make([]byte, 4+l+len(data))
		//#nosec G115 // safe: byte extraction from upper bits
		expected[0] = byte(l >> 24)
		//#nosec G115 // safe: byte extraction from upper bits
		expected[1] = byte(l >> 16)
		//#nosec G115 // safe: byte extraction from upper bits
		expected[2] = byte(l >> 8)
		//#nosec G115 // safe: byte extraction from lower bits
		expected[3] = byte(l)
		copy(expected[4:], prefix)
		copy(expected[4+l:], data)

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

		result := canonical.DomainSeparate(ctx, data)
		expectedLen := 4 + len(ctx) + len(data)

		if len(result) != expectedLen {
			rt.Fatalf(
				"DomainSeparate length = %d, want %d",
				len(result), expectedLen,
			)
		}
	})
}
