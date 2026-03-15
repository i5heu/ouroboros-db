package auth_test // A

import (
	"bytes"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"pgregory.net/rapid"
)

func TestPropertyEmptyCertBundleRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		_, err := ca.VerifyPeerCert(
			nil, nil, nil, nil, nil, nil, nil, nil,
		)
		if err != auth.ErrNoCerts {
			rt.Fatalf("expected ErrNoCerts, got %v", err)
		}
	})
}

func TestPropertySignatureCountMismatch(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)
		s.certData.validFrom = s.now - 100
		s.certData.validUntil = s.now + 600
		cert, _ := s.certData.toNodeCert()

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{cert},
			nil,
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrSignatureCountMismatch {
			rt.Fatalf("expected ErrSignatureCountMismatch, got %v", err)
		}
	})
}

func TestPropertyUnknownIssuerRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		nodeKP := getKeyFromPool(rt)
		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		cd.issuerHash = "unknown-issuer-hash-00000000000000000000001"
		cert, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		tls := genTLSSession().Draw(rt, "tls")
		proof := auth.NewDelegationProof(
			tls.certPubKeyHash,
			tls.exporterBinding,
			tls.transcriptHash,
			tls.x509Fingerprint,
			[]byte("bundle-hash"),
			now-5, now+auth.MaxDelegationTTL-10,
		)

		_, err = ca.VerifyPeerCert(
			[]auth.NodeCertLike{cert},
			[][]byte{[]byte("sig")},
			proof,
			[]byte("del-sig"),
			tls.certPubKeyHash,
			tls.exporterBinding,
			tls.x509Fingerprint,
			tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyExpiredCertFiltered(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		now := s.now
		cd := genCertData(s.nodeKP, now).Draw(rt, "expiredCert")
		cd.issuerHash = s.adminCA.Hash()
		cd.validFrom = now - 3600
		cd.validUntil = now - 1800
		expired, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		canonical, _ := auth.CanonicalNodeCert(expired)
		msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		sig, _ := s.adminKP.ac.Sign(msg)

		_, err = s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{expired},
			[][]byte{sig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyNotYetValidCertFiltered(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		now := s.now
		cd := genCertData(s.nodeKP, now).Draw(rt, "futureCert")
		cd.issuerHash = s.adminCA.Hash()
		cd.validFrom = now + 3600
		cd.validUntil = now + 7200
		future, err := cd.toNodeCert()
		if err != nil {
			rt.Fatalf("toNodeCert: %v", err)
		}

		canonical, _ := auth.CanonicalNodeCert(future)
		msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		sig, _ := s.adminKP.ac.Sign(msg)

		_, err = s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{future},
			[][]byte{sig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyRevokedCAFiltersCerts(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		err := s.ca.RevokeAdminCA(s.adminCA.Hash())
		if err != nil {
			rt.Fatalf("RevokeAdminCA: %v", err)
		}

		_, err = s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyRevokedNodeFiltersAllCerts(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		nodeID := s.cert.NodeID()
		err := s.ca.RevokeNode(nodeID)
		if err != nil {
			rt.Fatalf("RevokeNode: %v", err)
		}

		_, err = s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyInvalidCASigRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		badSig := rapid.SliceOfN(
			rapid.Byte(), 32, 64,
		).Draw(rt, "badSig")

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{badSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertyMismatchedNodeIDRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP := getKeyFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, _ := auth.NewAdminCA(adminKP.combined)

		node1KP, node2KP := getTwoDifferentKeysFromPool(rt)
		now := time.Now().Unix()

		cd1 := genCertData(node1KP, now).Draw(rt, "cert1")
		cd1.issuerHash = adminCA.Hash()
		cd1.validFrom = now - 3600
		cd1.validUntil = now + 3600
		cert1, _ := cd1.toNodeCert()
		canonical1, _ := auth.CanonicalNodeCert(cert1)
		msg1 := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical1)
		sig1, _ := adminKP.ac.Sign(msg1)

		cd2 := genCertData(node2KP, now).Draw(rt, "cert2")
		cd2.issuerHash = adminCA.Hash()
		cd2.validFrom = now - 3600
		cd2.validUntil = now + 3600
		cert2, _ := cd2.toNodeCert()
		canonical2, _ := auth.CanonicalNodeCert(cert2)
		msg2 := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical2)
		sig2, _ := adminKP.ac.Sign(msg2)

		proof := auth.NewDelegationProof(
			[]byte("hash"), []byte("exp"), []byte("trans"),
			[]byte("x509"), []byte("bundle"),
			now-5, now+auth.MaxDelegationTTL-10,
		)

		_, err := ca.VerifyPeerCert(
			[]auth.NodeCertLike{cert1, cert2},
			[][]byte{sig1, sig2},
			proof,
			[]byte("del-sig"),
			[]byte("hash"),
			[]byte("exp"),
			[]byte("x509"),
			[]byte("trans"),
		)
		if err != auth.ErrMismatchedNodeID {
			rt.Fatalf("expected ErrMismatchedNodeID, got %v", err)
		}
	})
}

func TestPropertyWrongDelegationSigRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		badDelSig := rapid.SliceOfN(
			rapid.Byte(), 32, 64,
		).Draw(rt, "badDelSig")

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			badDelSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrInvalidDelegationSig {
			rt.Fatalf("expected ErrInvalidDelegationSig, got %v", err)
		}
	})
}

func TestPropertyTLSCertHashMismatchRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		wrongHash := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(rt, "wrongHash")
		if bytes.Equal(wrongHash, s.tls.certPubKeyHash) {
			wrongHash[0] ^= 0xFF
		}

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			wrongHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrTLSBindingMismatch {
			rt.Fatalf("expected ErrTLSBindingMismatch, got %v", err)
		}
	})
}

func TestPropertyX509FingerprintMismatchRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		wrongFP := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(rt, "wrongFP")
		if bytes.Equal(wrongFP, s.tls.x509Fingerprint) {
			wrongFP[0] ^= 0xFF
		}

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			wrongFP,
			s.tls.transcriptHash,
		)
		if err != auth.ErrTLSBindingMismatch {
			rt.Fatalf("expected ErrTLSBindingMismatch, got %v", err)
		}
	})
}

func TestPropertyBundleHashMismatchRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		wrongBundle := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(rt, "wrongBundle")
		if bytes.Equal(wrongBundle, s.proof.NodeCertBundleHash()) {
			wrongBundle[0] ^= 0xFF
		}

		tampered := auth.NewDelegationProof(
			s.proof.TLSCertPubKeyHash(),
			s.proof.TLSExporterBinding(),
			s.proof.TLSTranscriptHash(),
			s.proof.X509Fingerprint(),
			wrongBundle,
			s.proof.NotBefore(),
			s.proof.NotAfter(),
		)

		canon, _ := auth.CanonicalDelegationProof(tampered)
		msg := auth.DomainSeparate(auth.CTXNodeDelegationV1, canon)
		sig, _ := s.nodeKP.ac.Sign(msg)

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			tampered,
			sig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrBundleHashMismatch {
			rt.Fatalf("expected ErrBundleHashMismatch, got %v", err)
		}
	})
}

func TestPropertyDelegationExpiredRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		now := time.Now().Unix()
		expiredProof := auth.NewDelegationProof(
			s.proof.TLSCertPubKeyHash(),
			s.proof.TLSExporterBinding(),
			s.proof.TLSTranscriptHash(),
			s.proof.X509Fingerprint(),
			s.proof.NodeCertBundleHash(),
			now-600,
			now-300,
		)

		canon, _ := auth.CanonicalDelegationProof(expiredProof)
		msg := auth.DomainSeparate(auth.CTXNodeDelegationV1, canon)
		sig, _ := s.nodeKP.ac.Sign(msg)

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			expiredProof,
			sig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrDelegationExpired {
			rt.Fatalf("expected ErrDelegationExpired, got %v", err)
		}
	})
}

func TestPropertyDelegationTTLTooLong(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		now := time.Now().Unix()
		longTTL := auth.MaxDelegationTTL + 100
		longProof := auth.NewDelegationProof(
			s.proof.TLSCertPubKeyHash(),
			s.proof.TLSExporterBinding(),
			s.proof.TLSTranscriptHash(),
			s.proof.X509Fingerprint(),
			s.proof.NodeCertBundleHash(),
			now-10,
			now+longTTL,
		)

		canon, _ := auth.CanonicalDelegationProof(longProof)
		msg := auth.DomainSeparate(auth.CTXNodeDelegationV1, canon)
		sig, _ := s.nodeKP.ac.Sign(msg)

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			longProof,
			sig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrDelegationTooLong {
			rt.Fatalf("expected ErrDelegationTooLong, got %v", err)
		}
	})
}

func TestPropertyExporterBindingMismatchRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		wrongExp := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(rt, "wrongExp")
		if bytes.Equal(wrongExp, s.tls.exporterBinding) {
			wrongExp[0] ^= 0xFF
		}

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			wrongExp,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != auth.ErrTLSBindingMismatch {
			rt.Fatalf("expected ErrTLSBindingMismatch, got %v", err)
		}
	})
}

func TestPropertyTranscriptHashMismatchRejected(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		wrongTrans := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(rt, "wrongTrans")
		if bytes.Equal(wrongTrans, s.tls.transcriptHash) {
			wrongTrans[0] ^= 0xFF
		}

		_, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			wrongTrans,
		)
		if err != auth.ErrTLSBindingMismatch {
			rt.Fatalf("expected ErrTLSBindingMismatch, got %v", err)
		}
	})
}

func TestPropertyAdminScopeFromAdminCert(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		ctx, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != nil {
			rt.Fatalf("VerifyPeerCert: %v", err)
		}
		if ctx.EffectiveScope != auth.ScopeAdmin {
			rt.Fatalf(
				"scope = %v, want ScopeAdmin",
				ctx.EffectiveScope,
			)
		}
		if !ctx.HasValidAdminCert {
			rt.Fatal("should have HasValidAdminCert=true")
		}
	})
}

func TestPropertyUserScopeFromUserCert(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP, userKP, nodeKP := getThreeDifferentKeysFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, _ := auth.NewAdminCA(adminKP.combined)

		anchorMsg := auth.DomainSeparate(
			auth.CTXUserCAAnchorV1,
			userKP.combined,
		)
		anchorSig, _ := adminKP.ac.Sign(anchorMsg)
		if err := ca.AddUserPubKey(
			userKP.combined, anchorSig, adminCA.Hash(),
		); err != nil {
			rt.Fatalf("AddUserPubKey: %v", err)
		}

		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		userCAHash := hashFromKey(userKP.pubKey)
		cd.issuerHash = userCAHash
		cd.validFrom = now - 3600
		cd.validUntil = now + 3600
		cert, _ := cd.toNodeCert()

		canonical, _ := auth.CanonicalNodeCert(cert)
		msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		caSig, _ := userKP.ac.Sign(msg)

		certs := []auth.NodeCertLike{cert}
		bundleBytes, _ := auth.CanonicalNodeCertBundle(certs)
		bundleHash := sha256.Sum256(bundleBytes)

		tls := genTLSSession().Draw(rt, "tls")
		proofNoExp := auth.NewDelegationProof(
			tls.certPubKeyHash, nil,
			tls.transcriptHash, tls.x509Fingerprint,
			bundleHash[:], now-5, now+auth.MaxDelegationTTL-10,
		)
		expCtx, _ := auth.CanonicalDelegationProofForExporter(proofNoExp)
		exporter := deriveTestExporter(expCtx)

		proof := auth.NewDelegationProof(
			tls.certPubKeyHash, exporter,
			tls.transcriptHash, tls.x509Fingerprint,
			bundleHash[:], now-5, now+auth.MaxDelegationTTL-10,
		)
		delCanon, _ := auth.CanonicalDelegationProof(proof)
		delMsg := auth.DomainSeparate(auth.CTXNodeDelegationV1, delCanon)
		delSig, _ := nodeKP.ac.Sign(delMsg)

		ctx, err := ca.VerifyPeerCert(
			certs, [][]byte{caSig}, proof, delSig,
			tls.certPubKeyHash, exporter,
			tls.x509Fingerprint, tls.transcriptHash,
		)
		if err != nil {
			rt.Fatalf("VerifyPeerCert: %v", err)
		}
		if ctx.EffectiveScope != auth.ScopeUser {
			rt.Fatalf("scope = %v, want ScopeUser", ctx.EffectiveScope)
		}
		if ctx.HasValidAdminCert {
			rt.Fatal("should not have HasValidAdminCert")
		}
		if len(ctx.AllowedUserCAOwners) == 0 {
			rt.Fatal("AllowedUserCAOwners should not be empty")
		}
	})
}

func TestPropertyAdminDominatesUserScope(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP, userKP, nodeKP := getThreeDifferentKeysFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, _ := auth.NewAdminCA(adminKP.combined)

		anchorMsg := auth.DomainSeparate(
			auth.CTXUserCAAnchorV1,
			userKP.combined,
		)
		anchorSig, _ := adminKP.ac.Sign(anchorMsg)
		if err := ca.AddUserPubKey(
			userKP.combined, anchorSig, adminCA.Hash(),
		); err != nil {
			rt.Fatalf("AddUserPubKey: %v", err)
		}
		userCAHash := hashFromKey(userKP.pubKey)

		now := time.Now().Unix()

		adminCert := genCertData(nodeKP, now).Draw(rt, "adminCert")
		adminCert.issuerHash = adminCA.Hash()
		adminCert.validFrom = now - 3600
		adminCert.validUntil = now + 3600
		adminCertInst, _ := adminCert.toNodeCert()
		adminCanon, _ := auth.CanonicalNodeCert(adminCertInst)
		adminMsg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, adminCanon)
		adminSig, _ := adminKP.ac.Sign(adminMsg)

		userCert := genCertData(nodeKP, now).Draw(rt, "userCert")
		userCert.issuerHash = userCAHash
		userCert.validFrom = now - 3600
		userCert.validUntil = now + 3600
		userCertInst, _ := userCert.toNodeCert()
		userCanon, _ := auth.CanonicalNodeCert(userCertInst)
		userMsg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, userCanon)
		userSig, _ := userKP.ac.Sign(userMsg)

		certs := []auth.NodeCertLike{adminCertInst, userCertInst}
		bundleBytes, _ := auth.CanonicalNodeCertBundle(certs)
		bundleHash := sha256.Sum256(bundleBytes)

		tls := genTLSSession().Draw(rt, "tls")
		proofNoExp := auth.NewDelegationProof(
			tls.certPubKeyHash, nil,
			tls.transcriptHash, tls.x509Fingerprint,
			bundleHash[:], now-5, now+auth.MaxDelegationTTL-10,
		)
		expCtx, _ := auth.CanonicalDelegationProofForExporter(proofNoExp)
		exporter := deriveTestExporter(expCtx)

		proof := auth.NewDelegationProof(
			tls.certPubKeyHash, exporter,
			tls.transcriptHash, tls.x509Fingerprint,
			bundleHash[:], now-5, now+auth.MaxDelegationTTL-10,
		)
		delCanon, _ := auth.CanonicalDelegationProof(proof)
		delMsg := auth.DomainSeparate(auth.CTXNodeDelegationV1, delCanon)
		delSig, _ := nodeKP.ac.Sign(delMsg)

		ctx, err := ca.VerifyPeerCert(
			certs, [][]byte{adminSig, userSig}, proof, delSig,
			tls.certPubKeyHash, exporter,
			tls.x509Fingerprint, tls.transcriptHash,
		)
		if err != nil {
			rt.Fatalf("VerifyPeerCert: %v", err)
		}
		if ctx.EffectiveScope != auth.ScopeAdmin {
			rt.Fatalf(
				"scope = %v, want ScopeAdmin (admin should dominate)",
				ctx.EffectiveScope,
			)
		}
	})
}

func TestPropertyRevokePreventsReadd(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP := getKeyFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, _ := auth.NewAdminCA(adminKP.combined)

		err := ca.RevokeAdminCA(adminCA.Hash())
		if err != nil {
			rt.Fatalf("RevokeAdminCA: %v", err)
		}

		err = ca.AddAdminPubKey(adminKP.combined)
		if err != auth.ErrCARevoked {
			rt.Fatalf("expected ErrCARevoked, got %v", err)
		}
	})
}

func TestPropertyAnchorAdminRemovalInvalidatesUserCA(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		ca := auth.NewCarrierAuth(testLogger())

		adminKP, userKP, nodeKP := getThreeDifferentKeysFromPool(rt)
		if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
			rt.Fatalf("AddAdminPubKey: %v", err)
		}
		adminCA, _ := auth.NewAdminCA(adminKP.combined)

		anchorMsg := auth.DomainSeparate(
			auth.CTXUserCAAnchorV1,
			userKP.combined,
		)
		anchorSig, _ := adminKP.ac.Sign(anchorMsg)
		if err := ca.AddUserPubKey(
			userKP.combined, anchorSig, adminCA.Hash(),
		); err != nil {
			rt.Fatalf("AddUserPubKey: %v", err)
		}
		userCAHash := hashFromKey(userKP.pubKey)

		_ = ca.RemoveAdminPubKey(adminCA.Hash())

		now := time.Now().Unix()
		cd := genCertData(nodeKP, now).Draw(rt, "certData")
		cd.issuerHash = userCAHash
		cd.validFrom = now - 3600
		cd.validUntil = now + 3600
		cert, _ := cd.toNodeCert()

		canonical, _ := auth.CanonicalNodeCert(cert)
		msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
		caSig, _ := userKP.ac.Sign(msg)

		certs := []auth.NodeCertLike{cert}
		bundleBytes, _ := auth.CanonicalNodeCertBundle(certs)
		bundleHash := sha256.Sum256(bundleBytes)

		tls := genTLSSession().Draw(rt, "tls")
		proof := auth.NewDelegationProof(
			tls.certPubKeyHash, tls.exporterBinding,
			tls.transcriptHash, tls.x509Fingerprint,
			bundleHash[:], now-5, now+auth.MaxDelegationTTL-10,
		)
		delCanon, _ := auth.CanonicalDelegationProof(proof)
		delMsg := auth.DomainSeparate(auth.CTXNodeDelegationV1, delCanon)
		delSig, _ := nodeKP.ac.Sign(delMsg)

		_, err := ca.VerifyPeerCert(
			certs, [][]byte{caSig}, proof, delSig,
			tls.certPubKeyHash, tls.exporterBinding,
			tls.x509Fingerprint, tls.transcriptHash,
		)
		if err != auth.ErrNoValidCerts {
			rt.Fatalf("expected ErrNoValidCerts, got %v", err)
		}
	})
}

func TestPropertySuccessfulVerification(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(rt *rapid.T) {
		s := buildAdminScenario(rt)

		ctx, err := s.ca.VerifyPeerCert(
			[]auth.NodeCertLike{s.cert},
			[][]byte{s.caSig},
			s.proof,
			s.delSig,
			s.tls.certPubKeyHash,
			s.tls.exporterBinding,
			s.tls.x509Fingerprint,
			s.tls.transcriptHash,
		)
		if err != nil {
			rt.Fatalf("VerifyPeerCert: %v", err)
		}

		expectedNID := s.cert.NodeID()
		if ctx.NodeID != expectedNID {
			rt.Fatalf(
				"NodeID mismatch: got %v, want %v",
				ctx.NodeID, expectedNID,
			)
		}
		if ctx.EffectiveScope != auth.ScopeAdmin {
			rt.Fatalf("scope = %v, want ScopeAdmin", ctx.EffectiveScope)
		}
	})
}

func hashFromKey(pubKey keys.PublicKey) string {
	signBytes, _ := pubKey.MarshalBinarySign()
	h := sha256.Sum256(signBytes)
	return hexEncodeToString(h[:])
}

func hexEncodeToString(b []byte) string {
	const hextable = "0123456789abcdef"
	dst := make([]byte, len(b)*2)
	for i, v := range b {
		dst[i*2] = hextable[v>>4]
		dst[i*2+1] = hextable[v&0x0f]
	}
	return string(dst)
}
