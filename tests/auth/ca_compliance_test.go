package auth_test // A

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-db/pkg/auth"
	"pgregory.net/rapid"
)

func TestPropertyAdminCAHashDerivation(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		adminKP := getKeyFromPool(rt)
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		signBytes, err := adminKP.pubKey.MarshalBinarySign()
		if err != nil {
			rt.Fatalf("MarshalBinarySign: %v", err)
		}
		expectedHash := sha256.Sum256(signBytes)
		expectedHashStr := hex.EncodeToString(expectedHash[:])

		if adminCA.Hash() != expectedHashStr {
			rt.Fatalf(
				"Hash = %q, want %q",
				adminCA.Hash(), expectedHashStr,
			)
		}
	})
}

func TestPropertyAdminCAPubKeyRoundtrip(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		adminKP := getKeyFromPool(rt)
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		pubKey := adminCA.PubKey()
		if len(pubKey) == 0 {
			rt.Fatal("PubKey is empty")
		}

		adminCA2, err := auth.NewAdminCA(pubKey)
		if err != nil {
			rt.Fatalf("NewAdminCA from roundtrip: %v", err)
		}
		if adminCA.Hash() != adminCA2.Hash() {
			rt.Fatal("Hash mismatch after roundtrip")
		}
	})
}

func TestPropertyAdminCAVerifyNodeCert(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		adminKP := getKeyFromPool(rt)
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		nodeKP := getKeyFromPool(rt)
		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		cd.issuerHash = adminCA.Hash()
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		canonical, err := auth.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("CanonicalNodeCert: %v", err)
		}
		msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		sig, err := adminKP.ac.Sign(msg)
		if err != nil {
			rt.Fatalf("Sign: %v", err)
		}

		nodeID, err := adminCA.VerifyNodeCert(cert, sig)
		if err != nil {
			rt.Fatalf("VerifyNodeCert: %v", err)
		}

		expectedNID := cert.NodeID()
		if nodeID != expectedNID {
			rt.Fatalf(
				"NodeID mismatch: got %v, want %v",
				nodeID, expectedNID,
			)
		}
	})
}

func TestPropertyAdminCAInvalidSigRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		adminKP := getKeyFromPool(rt)
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		nodeKP := getKeyFromPool(rt)
		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		cd.issuerHash = adminCA.Hash()
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		badSig := rapid.SliceOfN(rapid.Byte(), 32, 64).Draw(rt, "badSig")

		_, err = adminCA.VerifyNodeCert(cert, badSig)
		if err != auth.ErrInvalidCASignature {
			rt.Fatalf(
				"expected ErrInvalidCASignature, got %v",
				err,
			)
		}
	})
}

func TestPropertyAdminCAHashUnique(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		admin1, admin2 := getTwoDifferentKeysFromPool(rt)

		ca1, err := auth.NewAdminCA(admin1.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA1: %v", err)
		}
		ca2, err := auth.NewAdminCA(admin2.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA2: %v", err)
		}

		if ca1.Hash() == ca2.Hash() {
			rt.Fatal("different keys should have different hashes")
		}
	})
}

func TestPropertyUserCARequiresValidAnchor(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP := getKeyFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		userKP := getKeyFromPool(rt)
		badAnchorSig := rapid.SliceOfN(
			rapid.Byte(), 32, 64,
		).Draw(rt, "badAnchorSig")

		err = ca.AddUserPubKey(
			userKP.combined, badAnchorSig, adminCA.Hash(),
		)
		if err != auth.ErrInvalidAnchorSig {
			rt.Fatalf(
				"expected ErrInvalidAnchorSig, got %v",
				err,
			)
		}
	})
}

func TestPropertyUserCAValidAnchorAccepted(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP := getKeyFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		userKP := getKeyFromPool(rt)
		signBytes, err := userKP.pubKey.MarshalBinarySign()
		if err != nil {
			rt.Fatalf("MarshalBinarySign: %v", err)
		}
		anchorMsg := auth.DomainSeparate(auth.CTXUserCAAnchorV1, signBytes)
		anchorSig, err := adminKP.ac.Sign(anchorMsg)
		if err != nil {
			rt.Fatalf("Sign anchor: %v", err)
		}

		err = ca.AddUserPubKey(
			userKP.combined, anchorSig, adminCA.Hash(),
		)
		if err != nil {
			rt.Fatalf("AddUserPubKey: %v", err)
		}
	})
}

func TestPropertySigDomainSeparation(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		adminKP := getKeyFromPool(rt)
		adminCA, err := auth.NewAdminCA(adminKP.combined)
		if err != nil {
			rt.Fatalf("NewAdminCA: %v", err)
		}

		nodeKP := getKeyFromPool(rt)
		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		cd.issuerHash = adminCA.Hash()
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		canonical, err := auth.CanonicalNodeCert(cert)
		if err != nil {
			rt.Fatalf("CanonicalNodeCert: %v", err)
		}

		expectedMsg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		sig, err := adminKP.ac.Sign(expectedMsg)
		if err != nil {
			rt.Fatalf("Sign: %v", err)
		}

		nodeID, err := adminCA.VerifyNodeCert(cert, sig)
		if err != nil {
			rt.Fatalf("VerifyNodeCert: %v", err)
		}
		if nodeID != cert.NodeID() {
			rt.Fatal("NodeID mismatch")
		}
	})
}
