package auth

import (
	"strings"
	"sync"
	"testing"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

func TestNewCarrierAuth(t *testing.T) { // A
	ca := NewCarrierAuth()
	if ca == nil {
		t.Fatal("expected non-nil CarrierAuth")
	}
}

func TestVerifyPeerCertAdmin(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	if err := ca.AddAdminPubKey(caPub); err != nil {
		t.Fatalf("AddAdminPubKey: %v", err)
	}

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	nodeID, scope, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}

	if scope != ScopeAdmin {
		t.Errorf(
			"scope = %v, want ScopeAdmin",
			scope,
		)
	}

	expected, _ := nodePub.NodeID()
	if nodeID != expected {
		t.Errorf(
			"nodeID = %v, want %v",
			nodeID, expected,
		)
	}
}

func TestVerifyPeerCertUser(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	if err := ca.AddUserPubKey(caPub); err != nil {
		t.Fatalf("AddUserPubKey: %v", err)
	}

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, scope, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}

	if scope != ScopeUser {
		t.Errorf(
			"scope = %v, want ScopeUser",
			scope,
		)
	}
}

func TestVerifyPeerCertPoPFailure( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, otherPub,
	)
	if err == nil {
		t.Fatal("expected PoP failure")
	}
	if !strings.Contains(
		err.Error(), "does not match",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertAcceptsSameSignDifferentKEM( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)
	otherAC := generateKeys(t)
	otherPub := pubKeyPtr(t, otherAC)

	ca := NewCarrierAuth()
	if err := ca.AddAdminPubKey(caPub); err != nil {
		t.Fatalf("AddAdminPubKey: %v", err)
	}

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	otherKEM, err := otherPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("marshal other KEM key: %v", err)
	}
	nodeSign, err := nodePub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("marshal node sign key: %v", err)
	}
	tlsMixedPub, err := keys.NewPublicKeyFromBinary(
		otherKEM, nodeSign,
	)
	if err != nil {
		t.Fatalf("build mixed TLS public key: %v", err)
	}

	_, scope, err := ca.VerifyPeerCert(
		cert, sig, tlsMixedPub,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}
	if scope != ScopeAdmin {
		t.Errorf("scope = %v, want ScopeAdmin", scope)
	}
}

func TestVerifyPeerCertRevokedNode( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	nodeID, _ := nodePub.NodeID()
	_ = ca.RevokeNode(nodeID)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal("expected revoked node error")
	}
	if !strings.Contains(
		err.Error(), "revoked",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertRevokedAdminCA( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	caHash, _ := computeCAHash(caPub)
	_ = ca.RevokeAdminCA(caHash)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal("expected revoked CA error")
	}
}

func TestVerifyPeerCertRevokedUserCA( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddUserPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	caHash, _ := computeCAHash(caPub)
	_ = ca.RevokeUserCA(caHash)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal("expected revoked CA error")
	}
}

func TestVerifyPeerCertUnknownCA( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal("expected unknown CA error")
	}
	if !strings.Contains(
		err.Error(), "no trusted CA",
	) {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyPeerCertBadSignature( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)

	_, _, err := ca.VerifyPeerCert(
		cert, []byte("bad"), nodePub,
	)
	if err == nil {
		t.Fatal(
			"expected error for bad signature",
		)
	}
}

func TestVerifyPeerCertNilGuards(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	carrier := NewCarrierAuth()
	_ = carrier.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	tests := []struct {
		name      string
		carrier   *CarrierAuth
		peerCert  *NodeCert
		tlsPubKey *keys.PublicKey
		wantErr   string
	}{
		{
			name:      "nil carrier",
			carrier:   nil,
			peerCert:  cert,
			tlsPubKey: nodePub,
			wantErr:   "carrier auth must not be nil",
		},
		{
			name:      "nil peer cert",
			carrier:   carrier,
			peerCert:  nil,
			tlsPubKey: nodePub,
			wantErr:   "peer cert must not be nil",
		},
		{
			name:      "nil tls key",
			carrier:   carrier,
			peerCert:  cert,
			tlsPubKey: nil,
			wantErr:   "TLS peer public key must not be nil",
		},
		{
			name:    "malformed cert nil node key",
			carrier: carrier,
			peerCert: &NodeCert{
				nodePubKey:   nil,
				issuerCAHash: cert.IssuerCAHash(),
			},
			tlsPubKey: nodePub,
			wantErr:   "peer cert node public key must not be nil",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := tc.carrier.VerifyPeerCert(
				tc.peerCert, sig, tc.tlsPubKey,
			)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(
				err.Error(), tc.wantErr,
			) {
				t.Fatalf(
					"error = %q, want contains %q",
					err.Error(), tc.wantErr,
				)
			}
		})
	}
}

func TestAdminPreferredOverUser( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)
	_ = ca.AddUserPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, scope, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err != nil {
		t.Fatalf("VerifyPeerCert: %v", err)
	}
	if scope != ScopeAdmin {
		t.Errorf(
			"scope = %v, want ScopeAdmin "+
				"(admin should take precedence)",
			scope,
		)
	}
}

func TestRemoveAdminPubKey(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	caHash, _ := computeCAHash(caPub)
	_ = ca.RemoveAdminPubKey(caHash)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal(
			"expected error after removing " +
				"admin CA",
		)
	}
}

func TestRemoveUserPubKey(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddUserPubKey(caPub)

	caHash, _ := computeCAHash(caPub)
	_ = ca.RemoveUserPubKey(caHash)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	_, _, err := ca.VerifyPeerCert(
		cert, sig, nodePub,
	)
	if err == nil {
		t.Fatal(
			"expected error after removing " +
				"user CA",
		)
	}
}

func TestConcurrentVerifyPeerCert( // A
	t *testing.T,
) {
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(caPub)

	cert := buildTestCert(t, caPub, nodePub)
	sig := signCert(t, caAC, cert)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := ca.VerifyPeerCert(
				cert, sig, nodePub,
			)
			if err != nil {
				t.Errorf(
					"concurrent verify: %v",
					err,
				)
			}
		}()
	}
	wg.Wait()
}

func TestRevokeNodeAppliesToBothScopes( // A
	t *testing.T,
) {
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(adminPub)
	_ = ca.AddUserPubKey(userPub)

	nodeID, _ := nodePub.NodeID()
	_ = ca.RevokeNode(nodeID)

	adminCert := buildTestCert(
		t, adminPub, nodePub,
	)
	adminSig := signCert(t, adminAC, adminCert)
	_, _, err := ca.VerifyPeerCert(
		adminCert, adminSig, nodePub,
	)
	if err == nil {
		t.Fatal(
			"revoked node should fail admin " +
				"verify",
		)
	}

	userCert := buildTestCert(
		t, userPub, nodePub,
	)
	userSig := signCert(t, userAC, userCert)
	_, _, err = ca.VerifyPeerCert(
		userCert, userSig, nodePub,
	)
	if err == nil {
		t.Fatal(
			"revoked node should fail user " +
				"verify",
		)
	}
}

func TestAddAdminPubKeyNil(t *testing.T) { // A
	ca := NewCarrierAuth()
	err := ca.AddAdminPubKey(nil)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestAddAdminPubKeyRejectsRevokedHash( // A
	t *testing.T,
) {
	adminAC := generateKeys(t)
	adminPub := pubKeyPtr(t, adminAC)
	adminHash, err := computeCAHash(adminPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	ca := NewCarrierAuth()
	if err := ca.RevokeAdminCA(adminHash); err != nil {
		t.Fatalf("RevokeAdminCA: %v", err)
	}

	err = ca.AddAdminPubKey(adminPub)
	if err == nil {
		t.Fatal("expected error for revoked admin CA hash")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("error = %q, want contains revoked", err)
	}
}

func TestAddUserPubKeyNil(t *testing.T) { // A
	ca := NewCarrierAuth()
	err := ca.AddUserPubKey(nil)
	if err == nil {
		t.Fatal("expected error for nil pubkey")
	}
}

func TestAddUserPubKeyRejectsRevokedHash( // A
	t *testing.T,
) {
	userAC := generateKeys(t)
	userPub := pubKeyPtr(t, userAC)
	userHash, err := computeCAHash(userPub)
	if err != nil {
		t.Fatalf("computeCAHash: %v", err)
	}

	ca := NewCarrierAuth()
	if err := ca.RevokeUserCA(userHash); err != nil {
		t.Fatalf("RevokeUserCA: %v", err)
	}

	err = ca.AddUserPubKey(userPub)
	if err == nil {
		t.Fatal("expected error for revoked user CA hash")
	}
	if !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("error = %q, want contains revoked", err)
	}
}

func TestMultipleAdminCAs(t *testing.T) { // A
	ca1AC := generateKeys(t)
	ca1Pub := pubKeyPtr(t, ca1AC)
	ca2AC := generateKeys(t)
	ca2Pub := pubKeyPtr(t, ca2AC)

	ca := NewCarrierAuth()
	_ = ca.AddAdminPubKey(ca1Pub)
	_ = ca.AddAdminPubKey(ca2Pub)

	for _, tc := range []struct {
		name  string
		caAC  *keys.AsyncCrypt
		caPub *keys.PublicKey
	}{
		{"ca1", ca1AC, ca1Pub},
		{"ca2", ca2AC, ca2Pub},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nodeAC := generateKeys(t)
			nodePub := pubKeyPtr(t, nodeAC)
			cert := buildTestCert(
				t, tc.caPub, nodePub,
			)
			sig := signCert(t, tc.caAC, cert)

			_, scope, err := ca.VerifyPeerCert(
				cert, sig, nodePub,
			)
			if err != nil {
				t.Fatalf(
					"VerifyPeerCert: %v", err,
				)
			}
			if scope != ScopeAdmin {
				t.Errorf(
					"scope = %v, want admin",
					scope,
				)
			}
		})
	}
}
