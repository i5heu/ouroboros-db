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
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	"github.com/i5heu/ouroboros-db/pkg/authfile"
)

type testAdminInfo struct {
	AC       *keys.AsyncCrypt
	PubBytes []byte
	CA       *auth.AdminCAImpl
	PubKEM   []byte
	PubSign  []byte
}

type testUserInfo struct {
	AC        *keys.AsyncCrypt
	PubBytes  []byte
	CA        *auth.AdminCAImpl
	AnchorSig []byte
	PubKEM    []byte
	PubSign   []byte
}

func setupTestAdminCA(t *testing.T) *testAdminInfo { // A
	t.Helper()
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	pub := ac.GetPublicKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	ca, err := auth.NewAdminCA(pubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("admin KEM pub: %v", err)
	}
	pubSign, err := pub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("admin Sign pub: %v", err)
	}
	return &testAdminInfo{
		AC: ac, PubBytes: pubBytes,
		CA: ca, PubKEM: pubKEM, PubSign: pubSign,
	}
}

func setupTestAnchoredUserCA( // A
	t *testing.T, admin *testAdminInfo,
) *testUserInfo {
	t.Helper()
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	pub := ac.GetPublicKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	userCA, err := auth.NewAdminCA(pubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA user hash: %v", err)
	}
	anchorMsg := canonical.DomainSeparate(
		auth.CTXUserCAAnchorV1, pubBytes,
	)
	anchorSig, err := admin.AC.Sign(anchorMsg)
	if err != nil {
		t.Fatalf("anchor sign: %v", err)
	}
	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		t.Fatalf("user KEM pub: %v", err)
	}
	pubSign, err := pub.MarshalBinarySign()
	if err != nil {
		t.Fatalf("user Sign pub: %v", err)
	}
	return &testUserInfo{
		AC: ac, PubBytes: pubBytes,
		CA: userCA, AnchorSig: anchorSig,
		PubKEM: pubKEM, PubSign: pubSign,
	}
}

func writeTestAdminCAKey( // A
	t *testing.T, path string, ac *keys.AsyncCrypt,
) {
	t.Helper()
	keyJSON, err := authfile.MarshalKeyJSON(ac)
	if err != nil {
		t.Fatalf("admin marshal: %v", err)
	}
	f := authfile.CAKeyFile{Type: "admin-ca", KeyJSON: keyJSON}
	data, err := authfile.MarshalCAKey(&f)
	if err != nil {
		t.Fatalf("MarshalCAKey: %v", err)
	}
	if err := authfile.WriteFile(path, data); err != nil {
		t.Fatalf("write admin: %v", err)
	}
}

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

	var admin *testAdminInfo
	t.Run("create_admin", func(t *testing.T) { // A
		admin = setupTestAdminCA(t)
		writeTestAdminCAKey(t, adminPath, admin.AC)
	})

	var user *testUserInfo
	t.Run("create_user_anchored", func(t *testing.T) { // A
		user = setupTestAnchoredUserCA(t, admin)
		userJSON, err := authfile.MarshalKeyJSON(user.AC)
		if err != nil {
			t.Fatalf("user marshal: %v", err)
		}
		userFile := authfile.CAKeyFile{
			Type:        "user-ca",
			KeyJSON:     userJSON,
			AnchorSig:   user.AnchorSig,
			AnchorAdmin: admin.CA.Hash(),
		}
		ud, _ := authfile.MarshalCAKey(&userFile)
		if err := authfile.WriteFile(userPath, ud); err != nil {
			t.Fatalf("write user: %v", err)
		}
	})

	t.Run("verify_roundtrip", func(t *testing.T) { // A
		_, uf, err := authfile.ReadCAKey(userPath)
		if err != nil {
			t.Fatalf("ReadCAKey user: %v", err)
		}
		if uf.Type != "user-ca" {
			t.Fatalf("type = %q, want user-ca", uf.Type)
		}
		if uf.AnchorAdmin != admin.CA.Hash() {
			t.Fatal("anchor admin hash mismatch")
		}

		// Verify the anchor signature is valid.
		anchorMsg := canonical.DomainSeparate(
			auth.CTXUserCAAnchorV1, user.PubBytes,
		)
		if !admin.AC.Verify(anchorMsg, uf.AnchorSig) {
			t.Fatal("anchor signature invalid after load")
		}
	})
}

type nodeCertTestCtx struct {
	adminPath  string
	nodePath   string
	admin      *testAdminInfo
	caAC       *keys.AsyncCrypt
	caHash     string
	caPubBytes []byte
	caPubKEM   []byte
	caPubSign  []byte
	nodeAC     *keys.AsyncCrypt
	caSig      []byte
	now        time.Time
	serial     []byte
	nonce      []byte
	nodeID     keys.NodeID
}

func nodeCertTestSetupAdmin( // A
	t *testing.T, ctx *nodeCertTestCtx,
) {
	t.Helper()
	ctx.admin = setupTestAdminCA(t)
	writeTestAdminCAKey(t, ctx.adminPath, ctx.admin.AC)
}

func nodeCertTestReadAdmin( // A
	t *testing.T, ctx *nodeCertTestCtx,
) {
	t.Helper()
	var err error
	ctx.caAC, _, err = authfile.ReadCAKey(ctx.adminPath)
	if err != nil {
		t.Fatalf("ReadCAKey: %v", err)
	}
	caPub := ctx.caAC.GetPublicKey()
	ctx.caPubBytes, err = auth.MarshalPubKeyBytes(&caPub)
	if err != nil {
		t.Fatalf("ca pub: %v", err)
	}
	ctx.caPubKEM, _ = caPub.MarshalBinaryKEM()
	ctx.caPubSign, _ = caPub.MarshalBinarySign()
	h := sha256.Sum256(
		ctx.caPubBytes[auth.KEMPublicKeySize:],
	)
	ctx.caHash = fmt.Sprintf("%x", h)
}

func nodeCertTestCreateAndSign( // A
	t *testing.T, ctx *nodeCertTestCtx,
) {
	t.Helper()
	var err error
	ctx.nodeAC, err = keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("node key: %v", err)
	}
	nodePub := ctx.nodeAC.GetPublicKey()
	ctx.now = time.Now()
	ctx.serial = make([]byte, 16)
	_, _ = rand.Read(ctx.serial)
	ctx.nonce = make([]byte, 16)
	_, _ = rand.Read(ctx.nonce)

	cert, err := auth.NewNodeCert(
		nodePub, ctx.caHash,
		ctx.now.Unix(), ctx.now.Add(24*time.Hour).Unix(),
		ctx.serial, ctx.nonce,
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	canonicalData, err := canonical.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	msg := canonical.DomainSeparate(
		auth.CTXNodeAdmissionV1, canonicalData,
	)
	ctx.caSig, err = ctx.caAC.Sign(msg)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Verify signature before writing.
	ca, err := auth.NewAdminCA(ctx.caPubBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	if _, err := ca.VerifyNodeCert(
		cert, ctx.caSig,
	); err != nil {
		t.Fatalf("VerifyNodeCert: %v", err)
	}
	ctx.nodeID = cert.NodeID()
}

func nodeCertTestWrite( // A
	t *testing.T, ctx *nodeCertTestCtx,
) {
	t.Helper()
	nodeJSON, err := authfile.MarshalKeyJSON(ctx.nodeAC)
	if err != nil {
		t.Fatalf("node marshal: %v", err)
	}
	nf := authfile.NodeCertFile{
		Type:         "node-cert",
		KeyJSON:      nodeJSON,
		CAPubKEM:     ctx.caPubKEM,
		CAPubSign:    ctx.caPubSign,
		CASignature:  ctx.caSig,
		IssuerCAHash: ctx.caHash,
		ValidFrom:    ctx.now.Unix(),
		ValidUntil:   ctx.now.Add(24 * time.Hour).Unix(),
		Serial:       ctx.serial,
		CertNonce:    ctx.nonce,
	}
	nd, _ := authfile.MarshalNodeCert(&nf)
	if err := authfile.WriteFile(
		ctx.nodePath, nd,
	); err != nil {
		t.Fatalf("write node: %v", err)
	}
}

func nodeCertTestVerify( // A
	t *testing.T, ctx *nodeCertTestCtx,
) {
	t.Helper()
	loadedAC, loadedFile, err := authfile.ReadNodeCert(
		ctx.nodePath,
	)
	if err != nil {
		t.Fatalf("ReadNodeCert: %v", err)
	}

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
	if nid != ctx.nodeID {
		t.Fatal("NodeID mismatch after roundtrip")
	}
}

func TestNodeCertSignAndVerify(t *testing.T) { // A
	dir := t.TempDir()
	var ctx nodeCertTestCtx
	ctx.adminPath = filepath.Join(dir, "admin.oukey")
	ctx.nodePath = filepath.Join(dir, "node.oucert")

	nodeCertTestSetupAdmin(t, &ctx)
	nodeCertTestReadAdmin(t, &ctx)
	nodeCertTestCreateAndSign(t, &ctx)
	nodeCertTestWrite(t, &ctx)
	nodeCertTestVerify(t, &ctx)
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
	canonicalData, _ := canonical.CanonicalNodeCert(cert)
	msg := canonical.DomainSeparate(
		auth.CTXNodeAdmissionV1, canonicalData,
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
	var admin *testAdminInfo
	t.Run("setup_admin", func(t *testing.T) { // A
		admin = setupTestAdminCA(t)
	})

	var user *testUserInfo
	t.Run("setup_user", func(t *testing.T) { // A
		user = setupTestAnchoredUserCA(t, admin)
	})

	t.Run("build_and_verify_chain", func(t *testing.T) { // A
		userJSON, err := authfile.MarshalKeyJSON(user.AC)
		if err != nil {
			t.Fatalf("user marshal: %v", err)
		}
		chain, err := authfile.BuildEmbeddedTrustChain(
			user.AC,
			&authfile.CAKeyFile{
				Type:               "user-ca",
				KeyJSON:            userJSON,
				AnchorSig:          user.AnchorSig,
				AnchorAdmin:        admin.CA.Hash(),
				AnchorAdminPubKEM:  admin.PubKEM,
				AnchorAdminPubSign: admin.PubSign,
			},
		)
		if err != nil {
			t.Fatalf("BuildEmbeddedTrustChain: %v", err)
		}
		if len(chain) != 2 {
			t.Fatalf("chain len = %d, want 2", len(chain))
		}
		if chain[0].Type != "admin-ca" {
			t.Fatalf(
				"chain[0].Type = %q, want admin-ca",
				chain[0].Type,
			)
		}
		if chain[1].Type != "user-ca" {
			t.Fatalf(
				"chain[1].Type = %q, want user-ca",
				chain[1].Type,
			)
		}
		if chain[1].AnchorAdmin != admin.CA.Hash() {
			t.Fatalf(
				"chain[1].AnchorAdmin = %q, want %q",
				chain[1].AnchorAdmin,
				admin.CA.Hash(),
			)
		}
	})
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
	var admin *testAdminInfo
	t.Run("setup_admin", func(t *testing.T) { // A
		admin = setupTestAdminCA(t)
	})

	var user *testUserInfo
	t.Run("setup_user", func(t *testing.T) { // A
		user = setupTestAnchoredUserCA(t, admin)
	})

	t.Run("add_and_verify_trust", func(t *testing.T) { // A
		trust := auth.NewCarrierAuth(nil)
		err := authfile.AddEmbeddedTrust(
			trust,
			&authfile.NodeCertFile{
				Type: "node-cert",
				Authorities: []authfile.EmbeddedCAFile{
					{
						Type:    "admin-ca",
						PubKEM:  admin.PubKEM,
						PubSign: admin.PubSign,
					},
					{
						Type:        "user-ca",
						PubKEM:      user.PubKEM,
						PubSign:     user.PubSign,
						AnchorSig:   user.AnchorSig,
						AnchorAdmin: admin.CA.Hash(),
					},
				},
			},
		)
		if err != nil {
			t.Fatalf("AddEmbeddedTrust: %v", err)
		}
		if err := trust.RemoveUserPubKey(
			user.CA.Hash(),
		); err != nil {
			t.Fatalf("RemoveUserPubKey: %v", err)
		}
		if err := trust.RemoveAdminPubKey(
			admin.CA.Hash(),
		); err != nil {
			t.Fatalf("RemoveAdminPubKey: %v", err)
		}
	})
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
