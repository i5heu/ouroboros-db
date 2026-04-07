package auth_test // A

import (
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	"pgregory.net/rapid"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

var (
	keyPool     []*keyPair
	keyPoolOnce sync.Once
	keyPoolMu   sync.RWMutex
)

const keyPoolSize = 20

type keyPair struct {
	ac       *keys.AsyncCrypt
	pubKey   keys.PublicKey
	kem      []byte
	sign     []byte
	combined []byte
}

func initKeyPool() {
	keyPoolMu.Lock()
	defer keyPoolMu.Unlock()
	if len(keyPool) > 0 {
		return
	}
	keyPool = make([]*keyPair, keyPoolSize)
	for i := 0; i < keyPoolSize; i++ {
		ac, err := keys.NewAsyncCrypt()
		if err != nil {
			panic(fmt.Sprintf("key generation failed: %v", err))
		}
		pub := ac.GetPublicKey()
		kem, _ := pub.MarshalBinaryKEM()
		sign, _ := pub.MarshalBinarySign()
		combined := make([]byte, len(kem)+len(sign))
		copy(combined, kem)
		copy(combined[len(kem):], sign)
		keyPool[i] = &keyPair{
			ac:       ac,
			pubKey:   pub,
			kem:      kem,
			sign:     sign,
			combined: combined,
		}
	}
}

func getKeyFromPool(t *rapid.T) *keyPair {
	keyPoolOnce.Do(initKeyPool)
	idx := rapid.IntRange(0, keyPoolSize-1).Draw(t, "keyIdx")
	keyPoolMu.RLock()
	defer keyPoolMu.RUnlock()
	return keyPool[idx]
}

func getTwoDifferentKeysFromPool(t *rapid.T) (*keyPair, *keyPair) {
	keyPoolOnce.Do(initKeyPool)
	idx1 := rapid.IntRange(0, keyPoolSize-2).Draw(t, "keyIdx1")
	idx2 := rapid.IntRange(idx1+1, keyPoolSize-1).Draw(t, "keyIdx2")
	keyPoolMu.RLock()
	defer keyPoolMu.RUnlock()
	return keyPool[idx1], keyPool[idx2]
}

func getThreeDifferentKeysFromPool(t *rapid.T) (*keyPair, *keyPair, *keyPair) {
	keyPoolOnce.Do(initKeyPool)
	if keyPoolSize < 3 {
		t.Fatalf("key pool too small: %d", keyPoolSize)
	}
	idx1 := rapid.IntRange(0, keyPoolSize-3).Draw(t, "keyIdx1")
	idx2 := rapid.IntRange(idx1+1, keyPoolSize-2).Draw(t, "keyIdx2")
	idx3 := rapid.IntRange(idx2+1, keyPoolSize-1).Draw(t, "keyIdx3")
	keyPoolMu.RLock()
	defer keyPoolMu.RUnlock()
	return keyPool[idx1], keyPool[idx2], keyPool[idx3]
}

type certData struct {
	kp         *keyPair
	issuerHash string
	validFrom  int64
	validUntil int64
	serial     []byte
	certNonce  []byte
}

func genCertData(kp *keyPair, now int64) *rapid.Generator[*certData] {
	return rapid.Custom(func(t *rapid.T) *certData {
		validFrom := rapid.Int64Range(now-3600, now+3600).Draw(t, "validFrom")
		validUntil := rapid.Int64Range(validFrom+1, validFrom+7200).Draw(t, "validUntil")
		serial := rapid.SliceOfN(rapid.Byte(), 4, 32).Draw(t, "serial")
		certNonce := rapid.SliceOfN(rapid.Byte(), 4, 32).Draw(t, "certNonce")
		issuerHash := rapid.StringOfN(
			rapid.RuneFrom([]rune("0123456789abcdef")),
			64, 64, 64,
		).Draw(t, "issuerHash")
		return &certData{
			kp:         kp,
			issuerHash: issuerHash,
			validFrom:  validFrom,
			validUntil: validUntil,
			serial:     serial,
			certNonce:  certNonce,
		}
	})
}

func (cd *certData) toNodeCert() (*auth.NodeCertImpl, error) {
	return auth.NewNodeCert(
		cd.kp.pubKey,
		cd.issuerHash,
		cd.validFrom,
		cd.validUntil,
		cd.serial,
		cd.certNonce,
	)
}

type tlsSession struct {
	certPubKeyHash  []byte
	exporterBinding []byte
	transcriptHash  []byte
	x509Fingerprint []byte
}

func genTLSSession() *rapid.Generator[*tlsSession] {
	return rapid.Custom(func(t *rapid.T) *tlsSession {
		return &tlsSession{
			certPubKeyHash: rapid.SliceOfN(
				rapid.Byte(), 32, 32,
			).Draw(t, "certPubKeyHash"),
			exporterBinding: rapid.SliceOfN(
				rapid.Byte(), 32, 32,
			).Draw(t, "exporterBinding"),
			transcriptHash: rapid.SliceOfN(
				rapid.Byte(), 32, 32,
			).Draw(t, "transcriptHash"),
			x509Fingerprint: rapid.SliceOfN(
				rapid.Byte(), 32, 32,
			).Draw(t, "x509Fingerprint"),
		}
	})
}

type delegationData struct {
	tls        *tlsSession
	bundleHash []byte
	notBefore  int64
	notAfter   int64
}

func genDelegationData(
	tls *tlsSession, now int64,
) *rapid.Generator[*delegationData] {
	return rapid.Custom(func(t *rapid.T) *delegationData {
		notBefore := rapid.Int64Range(now-60, now+60).Draw(t, "delNotBefore")
		maxTTL := auth.MaxDelegationTTL
		ttl := rapid.Int64Range(10, maxTTL).Draw(t, "delTTL")
		notAfter := notBefore + ttl
		bundleHash := rapid.SliceOfN(
			rapid.Byte(), 32, 32,
		).Draw(t, "bundleHash")
		return &delegationData{
			tls:        tls,
			bundleHash: bundleHash,
			notBefore:  notBefore,
			notAfter:   notAfter,
		}
	})
}

func (dd *delegationData) toDelegationProof() *auth.DelegationProofImpl {
	return auth.NewDelegationProof(
		dd.tls.certPubKeyHash,
		dd.tls.exporterBinding,
		dd.tls.transcriptHash,
		dd.tls.x509Fingerprint,
		dd.bundleHash,
		dd.notBefore,
		dd.notAfter,
	)
}

func deriveTestExporter(ctx []byte) []byte {
	payload := append([]byte(auth.ExporterLabel), ctx...)
	sum := sha256.Sum256(payload)
	return sum[:]
}

type fullAdminScenario struct {
	ca       interfaces.CarrierAuth
	adminCA  *auth.AdminCAImpl
	adminKP  *keyPair
	nodeKP   *keyPair
	cert     *auth.NodeCertImpl
	certData *certData
	caSig    []byte
	proof    *auth.DelegationProofImpl
	delData  *delegationData
	delSig   []byte
	tls      *tlsSession
	now      int64
}

func buildAdminScenario(t *rapid.T) *fullAdminScenario {
	now := time.Now().Unix()
	ca := auth.NewCarrierAuth(testLogger())

	adminKP := getKeyFromPool(t)
	if err := ca.AddAdminPubKey(adminKP.combined); err != nil {
		t.Fatalf("AddAdminPubKey: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminKP.combined)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	nodeKP := getKeyFromPool(t)
	cd := genCertData(nodeKP, now).Draw(t, "certData")
	cd.issuerHash = adminCA.Hash()
	cd.validFrom = now - 3600
	cd.validUntil = now + 3600
	cert, err := cd.toNodeCert()
	if err != nil {
		t.Fatalf("toNodeCert: %v", err)
	}

	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		t.Fatalf("CanonicalNodeCert: %v", err)
	}
	msg := auth.DomainSeparate(auth.CTXNodeAdmissionV1, canonical)
	caSig, err := adminKP.ac.Sign(msg)
	if err != nil {
		t.Fatalf("admin sign: %v", err)
	}

	tls := genTLSSession().Draw(t, "tls")
	certs := []auth.NodeCertLike{cert}
	bundleBytes, err := auth.CanonicalNodeCertBundle(certs)
	if err != nil {
		t.Fatalf("CanonicalNodeCertBundle: %v", err)
	}
	bundleHash := sha256.Sum256(bundleBytes)

	delData := &delegationData{
		tls:        tls,
		bundleHash: bundleHash[:],
		notBefore:  now - 5,
		notAfter:   now + auth.MaxDelegationTTL - 10,
	}
	proofNoExp := auth.NewDelegationProof(
		tls.certPubKeyHash,
		nil,
		tls.transcriptHash,
		tls.x509Fingerprint,
		bundleHash[:],
		delData.notBefore,
		delData.notAfter,
	)
	expCtx, err := auth.CanonicalDelegationProofForExporter(proofNoExp)
	if err != nil {
		t.Fatalf("CanonicalDelegationProofForExporter: %v", err)
	}
	exporter := deriveTestExporter(expCtx)
	delData.tls.exporterBinding = exporter

	proof := auth.NewDelegationProof(
		tls.certPubKeyHash,
		exporter,
		tls.transcriptHash,
		tls.x509Fingerprint,
		bundleHash[:],
		delData.notBefore,
		delData.notAfter,
	)

	delCanonical, err := auth.CanonicalDelegationProof(proof)
	if err != nil {
		t.Fatalf("CanonicalDelegationProof: %v", err)
	}
	delMsg := auth.DomainSeparate(auth.CTXNodeDelegationV1, delCanonical)
	delSig, err := nodeKP.ac.Sign(delMsg)
	if err != nil {
		t.Fatalf("node sign delegation: %v", err)
	}

	return &fullAdminScenario{
		ca:       ca,
		adminCA:  adminCA,
		adminKP:  adminKP,
		nodeKP:   nodeKP,
		cert:     cert,
		certData: cd,
		caSig:    caSig,
		proof:    proof,
		delData:  delData,
		delSig:   delSig,
		tls:      tls,
		now:      now,
	}
}
