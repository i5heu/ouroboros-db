package auth

import (
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// validParams builds a NodeCertParams with all fields
// valid, using the given key and caHash.
func validParams( // A
	t *testing.T,
	pub *keys.PublicKey,
	caHash CaHash,
) NodeCertParams {
	t.Helper()
	now := time.Now()
	return NodeCertParams{
		NodePubKey:   pub,
		IssuerCAHash: caHash,
		ValidFrom:    now.Add(-time.Hour),
		ValidUntil:   now.Add(time.Hour),
		Serial:       testSerial(t),
		RoleClaims:   ScopeAdmin,
		CertNonce:    testNonce(t),
	}
}

func TestNewNodeCert(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(
		validParams(t, pub, caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	if cert == nil {
		t.Fatal("expected non-nil cert")
	}
}

func TestNewNodeCertRejections(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cases := []struct {
		name   string
		modify func(*NodeCertParams)
	}{
		{
			name: "nil public key",
			modify: func(p *NodeCertParams) {
				p.NodePubKey = nil
			},
		},
		{
			name: "validUntil equals validFrom",
			modify: func(p *NodeCertParams) {
				now := time.Now()
				p.ValidFrom = now
				p.ValidUntil = now
			},
		},
		{
			name: "validUntil before validFrom",
			modify: func(p *NodeCertParams) {
				now := time.Now()
				p.ValidFrom = now
				p.ValidUntil = now.Add(
					-time.Second,
				)
			},
		},
		{
			name: "zero serial",
			modify: func(p *NodeCertParams) {
				p.Serial = [16]byte{}
			},
		},
		{
			name: "zero nonce",
			modify: func(p *NodeCertParams) {
				p.CertNonce = [32]byte{}
			},
		},
		{
			name: "invalid role",
			modify: func(p *NodeCertParams) {
				p.RoleClaims = TrustScope(99)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { // A
			params := validParams(t, pub, caHash)
			tc.modify(&params)
			_, err := NewNodeCert(params)
			if err == nil {
				t.Fatalf(
					"expected error for %s",
					tc.name,
				)
			}
		})
	}
}

func TestNewNodeCertAcceptsBothRoles( // A
	t *testing.T,
) {
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	for _, role := range []TrustScope{
		ScopeAdmin, ScopeUser,
	} {
		params := validParams(t, pub, caHash)
		params.RoleClaims = role
		cert, err := NewNodeCert(params)
		if err != nil {
			t.Fatalf(
				"NewNodeCert(%v): %v",
				role, err,
			)
		}
		if cert.RoleClaims() != role {
			t.Errorf(
				"RoleClaims() = %v, want %v",
				cert.RoleClaims(), role,
			)
		}
	}
}

func TestNodeCertAccessors(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	params := validParams(t, pub, caHash)
	cert, err := NewNodeCert(params)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	t.Run("CertVersion", func(t *testing.T) { // A
		if cert.CertVersion() != currentCertVersion {
			t.Errorf(
				"CertVersion() = %d, want %d",
				cert.CertVersion(),
				currentCertVersion,
			)
		}
	})

	t.Run("NodePubKey", func(t *testing.T) { // A
		if !cert.NodePubKey().Equal(pub) {
			t.Error(
				"NodePubKey() does not match input",
			)
		}
	})

	t.Run("IssuerCAHash", func(t *testing.T) { // A
		if !cert.IssuerCAHash().Equal(caHash) {
			t.Error(
				"IssuerCAHash() does not match input",
			)
		}
	})

	t.Run("ValidFrom", func(t *testing.T) { // A
		want := params.ValidFrom.UTC()
		got := cert.ValidFrom()
		if !got.Equal(want) {
			t.Errorf(
				"ValidFrom() = %v, want %v",
				got, want,
			)
		}
	})

	t.Run("ValidUntil", func(t *testing.T) { // A
		want := params.ValidUntil.UTC()
		got := cert.ValidUntil()
		if !got.Equal(want) {
			t.Errorf(
				"ValidUntil() = %v, want %v",
				got, want,
			)
		}
	})

	t.Run("Serial", func(t *testing.T) { // A
		if cert.Serial() != params.Serial {
			t.Error("Serial() does not match input")
		}
	})

	t.Run("RoleClaims", func(t *testing.T) { // A
		if cert.RoleClaims() != params.RoleClaims {
			t.Errorf(
				"RoleClaims() = %v, want %v",
				cert.RoleClaims(),
				params.RoleClaims,
			)
		}
	})

	t.Run("CertNonce", func(t *testing.T) { // A
		if cert.CertNonce() != params.CertNonce {
			t.Error(
				"CertNonce() does not match input",
			)
		}
	})
}

func TestNodeCertNodeID(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(
		validParams(t, pub, caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	nodeID, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID: %v", err)
	}

	expected, err := pub.NodeID()
	if err != nil {
		t.Fatalf("pub.NodeID: %v", err)
	}

	if nodeID != expected {
		t.Errorf(
			"NodeID = %v, want %v",
			nodeID, expected,
		)
	}
}

func TestNodeCertNodeIDStable(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(
		validParams(t, pub, caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	id1, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID call 1: %v", err)
	}

	id2, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID call 2: %v", err)
	}

	if id1 != id2 {
		t.Error("NodeID not stable across calls")
	}
}

func TestNodeCertDifferentKeysProduceDifferentIDs( // A
	t *testing.T,
) {
	ac1 := generateKeys(t)
	ac2 := generateKeys(t)
	caHash := HashBytes([]byte("test-ca"))

	cert1, err := NewNodeCert(
		validParams(t, pubKeyPtr(t, ac1), caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert 1: %v", err)
	}

	cert2, err := NewNodeCert(
		validParams(t, pubKeyPtr(t, ac2), caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert 2: %v", err)
	}

	id1, _ := cert1.NodeID()
	id2, _ := cert2.NodeID()

	if id1 == id2 {
		t.Error(
			"different keys should produce " +
				"different NodeIDs",
		)
	}
}

func TestNodeCertNodeIDIndependentOfCA( // A
	t *testing.T,
) {
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	caHash1 := HashBytes([]byte("ca-1"))
	caHash2 := HashBytes([]byte("ca-2"))

	p1 := validParams(t, nodePub, caHash1)
	p2 := validParams(t, nodePub, caHash2)

	cert1, err := NewNodeCert(p1)
	if err != nil {
		t.Fatalf("NewNodeCert 1: %v", err)
	}

	cert2, err := NewNodeCert(p2)
	if err != nil {
		t.Fatalf("NewNodeCert 2: %v", err)
	}

	id1, _ := cert1.NodeID()
	id2, _ := cert2.NodeID()

	if id1 != id2 {
		t.Error(
			"NodeID should be independent of CA",
		)
	}
}

func TestNodeCertNodeIDNotZero(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("ca"))

	cert, err := NewNodeCert(
		validParams(t, pub, caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	id, err := cert.NodeID()
	if err != nil {
		t.Fatalf("NodeID: %v", err)
	}

	if id == (keys.NodeID{}) {
		t.Error("NodeID should not be zero")
	}
}

func TestNodeCertHashDeterministic(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	params := validParams(t, pub, caHash)
	cert, err := NewNodeCert(params)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	h1, err := cert.Hash()
	if err != nil {
		t.Fatalf("Hash call 1: %v", err)
	}

	h2, err := cert.Hash()
	if err != nil {
		t.Fatalf("Hash call 2: %v", err)
	}

	if h1 != h2 {
		t.Error("Hash() not deterministic")
	}
}

func TestNodeCertHashNotZero(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	cert, err := NewNodeCert(
		validParams(t, pub, caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	h, err := cert.Hash()
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if h == (CaHash{}) {
		t.Error("Hash() should not be zero")
	}
}

func TestNodeCertHashDiffersForDifferentCerts( // A
	t *testing.T,
) {
	ac1 := generateKeys(t)
	ac2 := generateKeys(t)
	caHash := HashBytes([]byte("test-ca"))

	cert1, err := NewNodeCert(
		validParams(t, pubKeyPtr(t, ac1), caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert 1: %v", err)
	}

	cert2, err := NewNodeCert(
		validParams(t, pubKeyPtr(t, ac2), caHash),
	)
	if err != nil {
		t.Fatalf("NewNodeCert 2: %v", err)
	}

	h1, err := cert1.Hash()
	if err != nil {
		t.Fatalf("Hash 1: %v", err)
	}

	h2, err := cert2.Hash()
	if err != nil {
		t.Fatalf("Hash 2: %v", err)
	}

	if h1 == h2 {
		t.Error(
			"different certs should produce " +
				"different hashes",
		)
	}
}

func TestNodeCertValidFromStoredAsUTC(t *testing.T) { // A
	ac := generateKeys(t)
	pub := pubKeyPtr(t, ac)
	caHash := HashBytes([]byte("test-ca"))

	params := validParams(t, pub, caHash)
	loc := time.FixedZone("UTC+5", 5*60*60)
	params.ValidFrom = params.ValidFrom.In(loc)
	params.ValidUntil = params.ValidUntil.In(loc)

	cert, err := NewNodeCert(params)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	if cert.ValidFrom().Location() != time.UTC {
		t.Error("ValidFrom not stored as UTC")
	}
	if cert.ValidUntil().Location() != time.UTC {
		t.Error("ValidUntil not stored as UTC")
	}
}

func TestBuildTestCertHelper(t *testing.T) { // A
	caAC := generateKeys(t)
	caPub := pubKeyPtr(t, caAC)
	nodeAC := generateKeys(t)
	nodePub := pubKeyPtr(t, nodeAC)

	cert := buildTestCert(
		t, caPub, nodePub, ScopeAdmin,
	)
	if cert == nil {
		t.Fatal("buildTestCert returned nil")
	}
	if cert.RoleClaims() != ScopeAdmin {
		t.Errorf(
			"RoleClaims() = %v, want ScopeAdmin",
			cert.RoleClaims(),
		)
	}
}
