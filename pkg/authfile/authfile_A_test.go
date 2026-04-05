package authfile_test

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/authfile"
)

func TestAdminCAWriteReadRoundtrip( // A
	t *testing.T,
) {
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.oukey")

	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("NewAsyncCrypt: %v", err)
	}
	keyJSON, err := authfile.MarshalKeyJSON(ac)
	if err != nil {
		t.Fatalf("MarshalKeyJSON: %v", err)
	}

	f := authfile.CAKeyFile{
		Type:    "admin-ca",
		KeyJSON: keyJSON,
	}
	data, err := authfile.MarshalCAKey(&f)
	if err != nil {
		t.Fatalf("MarshalCAKey: %v", err)
	}
	if err := authfile.WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ac2, f2, err := authfile.ReadCAKey(path)
	if err != nil {
		t.Fatalf("ReadCAKey: %v", err)
	}
	if f2.Type != "admin-ca" {
		t.Fatalf("type = %q, want admin-ca", f2.Type)
	}

	// Verify the loaded key can sign and the
	// original key can verify.
	msg := []byte("test message")
	sig, err := ac2.Sign(msg)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ac.Verify(msg, sig) {
		t.Fatal("signature from loaded key invalid")
	}
}

func TestUserCAWriteReadRoundtrip( // A
	t *testing.T,
) {
	dir := t.TempDir()
	adminPath := filepath.Join(dir, "admin.oukey")
	userPath := filepath.Join(dir, "user.oukey")

	// Create admin CA.
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminJSON, err := authfile.MarshalKeyJSON(adminAC)
	if err != nil {
		t.Fatalf("admin marshal: %v", err)
	}
	adminFile := authfile.CAKeyFile{
		Type:    "admin-ca",
		KeyJSON: adminJSON,
	}
	ad, _ := authfile.MarshalCAKey(&adminFile)
	if err := authfile.WriteFile(adminPath, ad); err != nil {
		t.Fatalf("write admin: %v", err)
	}

	// Create user CA anchored to admin.
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, err := auth.MarshalPubKeyBytes(
		&adminPub,
	)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	userPub := userAC.GetPublicKey()
	userPubBytes, err := auth.MarshalPubKeyBytes(
		&userPub,
	)
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	anchorMsg := auth.DomainSeparate(
		auth.CTXUserCAAnchorV1, userPubBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatalf("anchor sign: %v", err)
	}

	userJSON, err := authfile.MarshalKeyJSON(userAC)
	if err != nil {
		t.Fatalf("user marshal: %v", err)
	}
	userFile := authfile.CAKeyFile{
		Type:        "user-ca",
		KeyJSON:     userJSON,
		AnchorSig:   anchorSig,
		AnchorAdmin: adminCA.Hash(),
	}
	ud, _ := authfile.MarshalCAKey(&userFile)
	if err := authfile.WriteFile(userPath, ud); err != nil {
		t.Fatalf("write user: %v", err)
	}

	// Read back and verify.
	_, uf, err := authfile.ReadCAKey(userPath)
	if err != nil {
		t.Fatalf("ReadCAKey user: %v", err)
	}
	if uf.Type != "user-ca" {
		t.Fatalf("type = %q, want user-ca", uf.Type)
	}
	if uf.AnchorAdmin != adminCA.Hash() {
		t.Fatal("anchor admin hash mismatch")
	}

	// Verify the anchor signature is valid.
	if !adminAC.Verify(anchorMsg, uf.AnchorSig) {
		t.Fatal("anchor signature invalid after load")
	}
}

func TestNodeCertSignAndVerify(t *testing.T) { // A
	dir := t.TempDir()
	adminPath := filepath.Join(dir, "admin.oukey")
	nodePath := filepath.Join(dir, "node.oucert")

	// 1. Create admin CA and write to disk.
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminJSON, err := authfile.MarshalKeyJSON(adminAC)
	if err != nil {
		t.Fatalf("admin marshal: %v", err)
	}
	af := authfile.CAKeyFile{
		Type:    "admin-ca",
		KeyJSON: adminJSON,
	}
	ad, _ := authfile.MarshalCAKey(&af)
	if err := authfile.WriteFile(adminPath, ad); err != nil {
		t.Fatalf("write admin: %v", err)
	}

	// 2. Read admin back from disk.
	caAC, _, err := authfile.ReadCAKey(adminPath)
	if err != nil {
		t.Fatalf("ReadCAKey: %v", err)
	}
	caPub := caAC.GetPublicKey()
	caPubBytes, err := auth.MarshalPubKeyBytes(&caPub)
	if err != nil {
		t.Fatalf("ca pub: %v", err)
	}
	caPubKEM, _ := caPub.MarshalBinaryKEM()
	caPubSign, _ := caPub.MarshalBinarySign()

	h := sha256.Sum256(
		caPubBytes[auth.KEMPublicKeySize:],
	)
	caHash := fmt.Sprintf("%x", h)

	// 3. Generate node key and cert.
	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("node key: %v", err)
	}
	nodePub := nodeAC.GetPublicKey()
	now := time.Now()
	serial := make([]byte, 16)
	_, _ = rand.Read(serial)
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	cert, err := auth.NewNodeCert(
		nodePub, caHash,
		now.Unix(), now.Add(24*time.Hour).Unix(),
		serial, nonce,
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	// 4. Sign the cert with the loaded CA key.
	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	msg := auth.DomainSeparate(
		auth.CTXNodeAdmissionV1, canonical,
	)
	caSig, err := caAC.Sign(msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// 5. Verify signature before writing.
	ca, err := auth.NewAdminCA(caPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	if _, err := ca.VerifyNodeCert(cert, caSig); err != nil {
		t.Fatalf("VerifyNodeCert: %v", err)
	}

	// 6. Write .oucert and read it back.
	nodeJSON, err := authfile.MarshalKeyJSON(nodeAC)
	if err != nil {
		t.Fatalf("node marshal: %v", err)
	}
	nf := authfile.NodeCertFile{
		Type:         "node-cert",
		KeyJSON:      nodeJSON,
		CAPubKEM:     caPubKEM,
		CAPubSign:    caPubSign,
		CASignature:  caSig,
		IssuerCAHash: caHash,
		ValidFrom:    now.Unix(),
		ValidUntil:   now.Add(24 * time.Hour).Unix(),
		Serial:       serial,
		CertNonce:    nonce,
	}
	nd, _ := authfile.MarshalNodeCert(&nf)
	if err := authfile.WriteFile(nodePath, nd); err != nil {
		t.Fatalf("write node: %v", err)
	}

	loadedAC, loadedFile, err := authfile.ReadNodeCert(
		nodePath,
	)
	if err != nil {
		t.Fatalf("ReadNodeCert: %v", err)
	}

	// 7. Verify the CA signature still works
	//    after reading from disk.
	loadedPub := loadedAC.GetPublicKey()
	loadedCert, err := auth.NewNodeCert(
		loadedPub,
		loadedFile.IssuerCAHash,
		loadedFile.ValidFrom,
		loadedFile.ValidUntil,
		loadedFile.Serial,
		loadedFile.CertNonce,
	)
	if err != nil {
		t.Fatalf("rebuild cert: %v", err)
	}

	caPub2, err := keys.NewPublicKeyFromBinary(
		loadedFile.CAPubKEM, loadedFile.CAPubSign,
	)
	if err != nil {
		t.Fatalf("ca pub from file: %v", err)
	}
	caPub2Bytes, err := auth.MarshalPubKeyBytes(caPub2)
	if err != nil {
		t.Fatalf("ca pub bytes: %v", err)
	}
	ca2, err := auth.NewAdminCA(caPub2Bytes)
	if err != nil {
		t.Fatalf("NewAdminCA from file: %v", err)
	}
	nid, err := ca2.VerifyNodeCert(
		loadedCert, loadedFile.CASignature,
	)
	if err != nil {
		t.Fatalf(
			"VerifyNodeCert after load: %v", err,
		)
	}
	if nid != cert.NodeID() {
		t.Fatal("NodeID mismatch after roundtrip")
	}
}

func TestNodeCertToIdentity(t *testing.T) { // A
	dir := t.TempDir()
	nodePath := filepath.Join(dir, "node.oucert")

	// Create admin + node cert on disk.
	adminAC, _ := keys.NewAsyncCrypt()
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, _ := auth.MarshalPubKeyBytes(
		&adminPub,
	)
	caPubKEM, _ := adminPub.MarshalBinaryKEM()
	caPubSign, _ := adminPub.MarshalBinarySign()
	h := sha256.Sum256(
		adminPubBytes[auth.KEMPublicKeySize:],
	)
	caHash := fmt.Sprintf("%x", h)

	nodeAC, _ := keys.NewAsyncCrypt()
	nodePub := nodeAC.GetPublicKey()
	now := time.Now()
	serial := make([]byte, 16)
	_, _ = rand.Read(serial)
	nonce := make([]byte, 16)
	_, _ = rand.Read(nonce)

	cert, _ := auth.NewNodeCert(
		nodePub, caHash,
		now.Unix(), now.Add(24*time.Hour).Unix(),
		serial, nonce,
	)
	canonical, _ := auth.CanonicalNodeCert(cert)
	msg := auth.DomainSeparate(
		auth.CTXNodeAdmissionV1, canonical,
	)
	caSig, _ := adminAC.Sign(msg)

	nodeJSON, _ := authfile.MarshalKeyJSON(nodeAC)
	nf := authfile.NodeCertFile{
		Type:         "node-cert",
		KeyJSON:      nodeJSON,
		CAPubKEM:     caPubKEM,
		CAPubSign:    caPubSign,
		CASignature:  caSig,
		IssuerCAHash: caHash,
		ValidFrom:    now.Unix(),
		ValidUntil:   now.Add(24 * time.Hour).Unix(),
		Serial:       serial,
		CertNonce:    nonce,
	}
	nd, _ := authfile.MarshalNodeCert(&nf)
	_ = authfile.WriteFile(nodePath, nd)

	// Read and convert to NodeIdentity.
	ac, f, err := authfile.ReadNodeCert(nodePath)
	if err != nil {
		t.Fatalf("ReadNodeCert: %v", err)
	}
	ni, err := authfile.NodeCertToIdentity(ac, f)
	if err != nil {
		t.Fatalf("NodeCertToIdentity: %v", err)
	}
	if ni.NodeID() != cert.NodeID() {
		t.Fatal("NodeID mismatch")
	}
}

func TestBuildEmbeddedTrustChainAdmin( // A
	t *testing.T,
) {
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminJSON, err := authfile.MarshalKeyJSON(adminAC)
	if err != nil {
		t.Fatalf("admin marshal: %v", err)
	}
	chain, err := authfile.BuildEmbeddedTrustChain(
		adminAC,
		&authfile.CAKeyFile{
			Type:    "admin-ca",
			KeyJSON: adminJSON,
		},
	)
	if err != nil {
		t.Fatalf("BuildEmbeddedTrustChain: %v", err)
	}
	if len(chain) != 1 {
		t.Fatalf("chain len = %d, want 1", len(chain))
	}
	if chain[0].Type != "admin-ca" {
		t.Fatalf("chain[0].Type = %q, want admin-ca", chain[0].Type)
	}
}

func TestBuildEmbeddedTrustChainUser( // A
	t *testing.T,
) {
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	adminPubKEM, err := adminPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("admin KEM pub: %v", err)
	}
	adminPubSign, err := adminPub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("admin Sign pub: %v", err)
	}

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	userPub := userAC.GetPublicKey()
	userPubBytes, err := auth.MarshalPubKeyBytes(&userPub)
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	anchorMsg := auth.DomainSeparate(
		auth.CTXUserCAAnchorV1,
		userPubBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatalf("anchor sign: %v", err)
	}
	userJSON, err := authfile.MarshalKeyJSON(userAC)
	if err != nil {
		t.Fatalf("user marshal: %v", err)
	}
	chain, err := authfile.BuildEmbeddedTrustChain(
		userAC,
		&authfile.CAKeyFile{
			Type:               "user-ca",
			KeyJSON:            userJSON,
			AnchorSig:          anchorSig,
			AnchorAdmin:        adminCA.Hash(),
			AnchorAdminPubKEM:  adminPubKEM,
			AnchorAdminPubSign: adminPubSign,
		},
	)
	if err != nil {
		t.Fatalf("BuildEmbeddedTrustChain: %v", err)
	}
	if len(chain) != 2 {
		t.Fatalf("chain len = %d, want 2", len(chain))
	}
	if chain[0].Type != "admin-ca" {
		t.Fatalf("chain[0].Type = %q, want admin-ca", chain[0].Type)
	}
	if chain[1].Type != "user-ca" {
		t.Fatalf("chain[1].Type = %q, want user-ca", chain[1].Type)
	}
	if chain[1].AnchorAdmin != adminCA.Hash() {
		t.Fatalf(
			"chain[1].AnchorAdmin = %q, want %q",
			chain[1].AnchorAdmin,
			adminCA.Hash(),
		)
	}
}

func TestAddEmbeddedTrustLegacyAdmin( // A
	t *testing.T,
) {
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	caPubKEM, err := adminPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("admin KEM pub: %v", err)
	}
	caPubSign, err := adminPub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("admin Sign pub: %v", err)
	}
	trust := auth.NewCarrierAuth(nil)
	err = authfile.AddEmbeddedTrust(trust, &authfile.NodeCertFile{
		Type:      "node-cert",
		CAPubKEM:  caPubKEM,
		CAPubSign: caPubSign,
	})
	if err != nil {
		t.Fatalf("AddEmbeddedTrust: %v", err)
	}
	if err := trust.RemoveAdminPubKey(adminCA.Hash()); err != nil {
		t.Fatalf("RemoveAdminPubKey: %v", err)
	}
}

func TestAddEmbeddedTrustUserChain( // A
	t *testing.T,
) {
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	adminPubKEM, err := adminPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("admin KEM pub: %v", err)
	}
	adminPubSign, err := adminPub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("admin Sign pub: %v", err)
	}

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	userPub := userAC.GetPublicKey()
	userPubBytes, err := auth.MarshalPubKeyBytes(&userPub)
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	userCA, err := auth.NewAdminCA(userPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA user hash: %v", err)
	}
	anchorMsg := auth.DomainSeparate(
		auth.CTXUserCAAnchorV1,
		userPubBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatalf("anchor sign: %v", err)
	}
	userPubKEM, err := userPub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("user KEM pub: %v", err)
	}
	userPubSign, err := userPub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("user Sign pub: %v", err)
	}
	trust := auth.NewCarrierAuth(nil)
	err = authfile.AddEmbeddedTrust(trust, &authfile.NodeCertFile{
		Type: "node-cert",
		Authorities: []authfile.EmbeddedCAFile{
			{
				Type:    "admin-ca",
				PubKEM:  adminPubKEM,
				PubSign: adminPubSign,
			},
			{
				Type:        "user-ca",
				PubKEM:      userPubKEM,
				PubSign:     userPubSign,
				AnchorSig:   anchorSig,
				AnchorAdmin: adminCA.Hash(),
			},
		},
	})
	if err != nil {
		t.Fatalf("AddEmbeddedTrust: %v", err)
	}
	if err := trust.RemoveUserPubKey(userCA.Hash()); err != nil {
		t.Fatalf("RemoveUserPubKey: %v", err)
	}
	if err := trust.RemoveAdminPubKey(adminCA.Hash()); err != nil {
		t.Fatalf("RemoveAdminPubKey: %v", err)
	}
}

func TestReadCAKeyRejectsNodeCert(t *testing.T) { // A
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.oukey")
	_ = os.WriteFile(path, []byte("garbage"), 0o600)
	_, _, err := authfile.ReadCAKey(path)
	if err == nil {
		t.Fatal("expected error for garbage file")
	}
}

func TestReadNodeCertRejectsCAKey(t *testing.T) { // A
	dir := t.TempDir()
	path := filepath.Join(dir, "admin.oukey")

	ac, _ := keys.NewAsyncCrypt()
	j, _ := authfile.MarshalKeyJSON(ac)
	f := authfile.CAKeyFile{Type: "admin-ca", KeyJSON: j}
	d, _ := authfile.MarshalCAKey(&f)
	_ = authfile.WriteFile(path, d)

	_, _, err := authfile.ReadNodeCert(path)
	if err == nil {
		t.Fatal("expected error reading CA as node cert")
	}
}
