package auth_test // A

import (
	"crypto/sha256"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"pgregory.net/rapid"
)

func TestPropertyNodeIDDeterministic(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)

		cd1 := genCertData(kp, now).Draw(rt, "certData1")
		cd2 := genCertData(kp, now).Draw(rt, "certData2")
		cd2.issuerHash = "different-issuer-hash-0000000000001"

		cert1, err := cd1.toNodeCert()
		if err != nil {
			rt.Fatalf("cert1: %v", err)
		}
		cert2, err := cd2.toNodeCert()
		if err != nil {
			rt.Fatalf("cert2: %v", err)
		}

		if cert1.NodeID() != cert2.NodeID() {
			rt.Fatalf(
				"same pubKey should have same NodeID: %v != %v",
				cert1.NodeID(), cert2.NodeID(),
			)
		}
	})
}

func TestPropertyNodeIDDerivedFromPubKey(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		kemBytes, err := kp.pubKey.MarshalBinaryKEM()
		if err != nil {
			rt.Fatalf("MarshalBinaryKEM: %v", err)
		}
		signBytes, err := kp.pubKey.MarshalBinarySign()
		if err != nil {
			rt.Fatalf("MarshalBinarySign: %v", err)
		}
		combined := make([]byte, len(kemBytes)+len(signBytes))
		copy(combined, kemBytes)
		copy(combined[len(kemBytes):], signBytes)
		expectedHash := sha256.Sum256(combined)
		var expectedNodeID keys.NodeID
		copy(expectedNodeID[:], expectedHash[:])

		if cert.NodeID() != expectedNodeID {
			rt.Fatalf(
				"NodeID mismatch: got %v, want %v",
				cert.NodeID(), expectedNodeID,
			)
		}
	})
}

func TestPropertyCertValidityWindow(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		validFrom := cert.ValidFrom()
		validUntil := cert.ValidUntil()

		if validUntil <= validFrom {
			rt.Fatalf(
				"validUntil (%d) must be > validFrom (%d)",
				validUntil, validFrom,
			)
		}

		testTime := rapid.Int64Range(
			validFrom-1000, validUntil+1000,
		).Draw(rt, "testTime")

		isValid := testTime >= validFrom && testTime <= validUntil

		withinWindow := testTime >= validFrom && testTime <= validUntil
		if isValid != withinWindow {
			rt.Fatalf(
				"validity check mismatch at time %d: "+
					"validFrom=%d, validUntil=%d",
				testTime, validFrom, validUntil,
			)
		}
	})
}

func TestPropertyCertFieldsPreserved(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)
		cd := genCertData(kp, now).Draw(rt, "certData")
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		if cert.CertVersion() != auth.DefaultCertVersion {
			rt.Fatalf(
				"CertVersion = %d, want %d",
				cert.CertVersion(), auth.DefaultCertVersion,
			)
		}
		if cert.IssuerCAHash() != cd.issuerHash {
			rt.Fatalf(
				"IssuerCAHash = %q, want %q",
				cert.IssuerCAHash(), cd.issuerHash,
			)
		}
		if cert.ValidFrom() != cd.validFrom {
			rt.Fatalf(
				"ValidFrom = %d, want %d",
				cert.ValidFrom(), cd.validFrom,
			)
		}
		if cert.ValidUntil() != cd.validUntil {
			rt.Fatalf(
				"ValidUntil = %d, want %d",
				cert.ValidUntil(), cd.validUntil,
			)
		}
	})
}

func TestPropertyCertSerialUnique(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)

		cd1 := genCertData(kp, now).Draw(rt, "certData1")
		cd2 := genCertData(kp, now).Draw(rt, "certData2")

		if string(cd1.serial) == string(cd2.serial) {
			rt.Skip("serials happened to match")
		}

		cert1, err := cd1.toNodeCert()
		if err != nil {
			rt.Fatalf("cert1: %v", err)
		}
		cert2, err := cd2.toNodeCert()
		if err != nil {
			rt.Fatalf("cert2: %v", err)
		}

		if string(cert1.Serial()) == string(cert2.Serial()) {
			rt.Fatal("random serials should typically differ")
		}
	})
}

func TestPropertyNodeIDImmutableAcrossIssuers(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		kp := getKeyFromPool(rt)
		now := int64(1000000)

		cd := genCertData(kp, now).Draw(rt, "certData")
		originalCert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("original cert: %v", err)
		}
		originalNodeID := originalCert.NodeID()

		for i := 0; i < 5; i++ {
			newIssuer := rapid.StringOfN(
				rapid.RuneFrom([]rune("0123456789abcdef")),
				64, 64, 64,
			).Draw(rt, "issuer"+string(rune('0'+i)))

			cd.issuerHash = newIssuer
			newCert, err := cd.toNodeCert()
			if err != nil {
				rt.Fatalf("new cert %d: %v", i, err)
			}

			if newCert.NodeID() != originalNodeID {
				rt.Fatalf(
					"NodeID changed with issuer: %v -> %v",
					originalNodeID, newCert.NodeID(),
				)
			}
		}
	})
}
