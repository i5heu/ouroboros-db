package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	ouroboros "github.com/i5heu/ouroboros-db"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	"github.com/i5heu/ouroboros-db/pkg/authfile"
)

func TestLoadCarrierAuthEmbeddedAdminTrust( // A
	t *testing.T,
) {
	dir := t.TempDir()
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
	h := sha256.Sum256(adminPubBytes[auth.KEMPublicKeySize:])
	caHash := fmt.Sprintf("%x", h)

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
		nodePub,
		caHash,
		now.Unix(),
		now.Add(24*time.Hour).Unix(),
		serial,
		nonce,
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	canonicalData, err := canonical.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("CanonicalNodeCert: %v", err)
	}
	msg := canonical.DomainSeparate(auth.CTXNodeAdmissionV1, canonicalData)
	caSig, err := adminAC.Sign(msg)
	if err != nil {
		t.Fatalf("sign cert: %v", err)
	}
	nodeJSON, err := authfile.MarshalKeyJSON(nodeAC)
	if err != nil {
		t.Fatalf("node marshal: %v", err)
	}
	nodePath := filepath.Join(dir, "node.oucert")
	nodeData, err := authfile.MarshalNodeCert(&authfile.NodeCertFile{
		Type:         "node-cert",
		KeyJSON:      nodeJSON,
		CAPubKEM:     caPubKEM,
		CAPubSign:    caPubSign,
		CASignature:  caSig,
		Authorities:  chain,
		IssuerCAHash: caHash,
		ValidFrom:    now.Unix(),
		ValidUntil:   now.Add(24 * time.Hour).Unix(),
		Serial:       serial,
		CertNonce:    nonce,
	})
	if err != nil {
		t.Fatalf("MarshalNodeCert: %v", err)
	}
	if err := os.WriteFile(nodePath, nodeData, 0o600); err != nil {
		t.Fatalf("write node cert: %v", err)
	}

	carrierAuth, err := loadCarrierAuth(
		&ouroboros.Config{
			Identity: ouroboros.IdentityConfig{
				NodeCertPath: nodePath,
			},
		},
		slog.Default(),
	)
	if err != nil {
		t.Fatalf("loadCarrierAuth: %v", err)
	}
	if err := carrierAuth.RemoveAdminPubKey(adminCA.Hash()); err != nil {
		t.Fatalf("RemoveAdminPubKey: %v", err)
	}
}
