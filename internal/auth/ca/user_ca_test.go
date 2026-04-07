package ca

import (
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	canonicalpkg "github.com/i5heu/ouroboros-db/internal/auth/canonical"
)

func mustUserCA( // A
	t *testing.T,
	adminAC *keys.AsyncCrypt,
	adminCA *AdminCAImpl,
) (*UserCAImpl, *keys.AsyncCrypt) {
	t.Helper()
	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatal(err)
	}
	userPub := userAC.GetPublicKey()
	pubKeyBytes, err := marshalPubKeyBytes(&userPub)
	if err != nil {
		t.Fatal(err)
	}

	// AdminCA signs the full UserCA public key as anchor.
	anchorMsg := canonicalpkg.DomainSeparate(
		CTXUserCAAnchorV1, pubKeyBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatal(err)
	}

	kem, err := userPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatal(err)
	}
	sign, err := userPub.MarshalBinarySign()
	if err != nil {
		t.Fatal(err)
	}
	combined := append(kem, sign...)
	user, err := NewUserCA(
		combined, anchorSig, adminCA.Hash(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return user, userAC
}

func TestNewUserCA(t *testing.T) { // A
	adminCA, adminAC := mustAdminCA(t)
	user, _ := mustUserCA(t, adminAC, adminCA)

	if user.Hash() == "" {
		t.Error("hash should not be empty")
	}
	if len(user.PubKey()) == 0 {
		t.Error("PubKey should not be empty")
	}
	if user.AnchorAdminHash() != adminCA.Hash() {
		t.Error("anchor admin hash mismatch")
	}
	if len(user.AnchorSig()) == 0 {
		t.Error("anchor sig should not be empty")
	}
}

func TestUserCAVerifyNodeCert(t *testing.T) { // A
	adminCA, adminAC := mustAdminCA(t)
	user, userAC := mustUserCA(t, adminAC, adminCA)

	nodeAC := mustKeyPair(t)
	nodePub := nodeAC.GetPublicKey()
	cert := mustNodeCert(t, nodePub, user.Hash())

	canonical, err := canonicalpkg.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatal(err)
	}
	msg := canonicalpkg.DomainSeparate(CTXNodeAdmissionV1, canonical)
	sig, err := userAC.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	nid, err := user.VerifyNodeCert(cert, sig)
	if err != nil {
		t.Fatalf("VerifyNodeCert: %v", err)
	}
	expectedNID, _ := nodePub.NodeID()
	if nid != expectedNID {
		t.Error("returned NodeID mismatch")
	}
}
