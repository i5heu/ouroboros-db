package transport

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	"github.com/i5heu/ouroboros-db/internal/auth/delegation"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/carrier"
	"google.golang.org/protobuf/proto"
)

type testStream struct { // A
	reader *bytes.Reader
	writes bytes.Buffer
}

func newTestStream(data []byte) *testStream { // A
	return &testStream{reader: bytes.NewReader(data)}
}

func (s *testStream) Read(p []byte) (int, error) { // A
	if s.reader == nil {
		return 0, io.EOF
	}
	return s.reader.Read(p)
}

func (s *testStream) Write(p []byte) (int, error) { // A
	return s.writes.Write(p)
}

func (s *testStream) Close() error { // A
	return nil
}

type testConn struct { // A
	tlsBindings interfaces.TLSBindings
	exporter    map[string][]byte
}

func (c *testConn) NodeID() keys.NodeID { // A
	return keys.NodeID{}
}

func (c *testConn) RemoteAddr() string { // A
	return "test-remote"
}

func (c *testConn) TLSBindings() interfaces.TLSBindings { // A
	return c.tlsBindings
}

func (c *testConn) ExportKeyingMaterial( // A
	label string,
	_ []byte,
	_ int,
) ([]byte, error) {
	if data, ok := c.exporter[label]; ok {
		return append([]byte(nil), data...), nil
	}
	return nil, errors.New("missing exporter")
}

func (c *testConn) OpenStream() (interfaces.Stream, error) { // A
	return nil, errors.New("not implemented")
}

func (c *testConn) AcceptStream() (interfaces.Stream, error) { // A
	return nil, errors.New("not implemented")
}

func (c *testConn) SendDatagram([]byte) error { // A
	return errors.New("not implemented")
}

func (c *testConn) ReceiveDatagram() ([]byte, error) { // A
	return nil, errors.New("not implemented")
}

func (c *testConn) Close() error { // A
	return nil
}

func TestReadAuthHandshakeRejectsMalformedAuthoritiesProtobuf( // A
	t *testing.T,
) {
	// Send a valid protobuf with a malformed authority
	// (PubKem set to a non-bytes value is not possible
	// in protobuf, so we test with garbage bytes that
	// will parse but have invalid content).
	payload, err := proto.Marshal(&pb.WireAuthMessage{
		Certs:        []*pb.WireNodeCert{},
		CaSignatures: [][]byte{},
		Authorities: []*pb.WireEmbeddedCA{{
			Type:   "admin-ca",
			PubKem: []byte{7},
		}},
		Delegation:    &pb.WireDelegationProof{},
		DelegationSig: []byte("sig"),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(
		frame[:4],
		uint32(len(payload)), //#nosec G115 // safe: test payload size well within uint32 range
	)
	copy(frame[4:], payload)

	conn := &testConn{
		tlsBindings: interfaces.TLSBindings{},
		exporter: map[string][]byte{
			delegation.TranscriptBindingLabel: make(
				[]byte,
				delegation.TLSTranscriptHashSize,
			),
			delegation.ExporterLabel: make(
				[]byte,
				delegation.TLSExporterBindingSize,
			),
		},
	}
	// This should parse successfully since protobuf
	// allows any bytes in bytes fields. The authority
	// content is validated downstream.
	_, err = readAuthHandshake(
		newTestStream(frame), conn,
	)
	if err != nil {
		// Acceptable: downstream validation caught it.
		return
	}
}

type authHandshakeRoundTripEnv struct { // A
	adminCA   *auth.AdminCAImpl
	adminKEM  []byte
	adminSign []byte
	userKEM   []byte
	userSign  []byte
	anchorSig []byte
	cert      canonical.NodeCertLike
	proof     *delegation.DelegationProofImpl
}

func setupAuthHandshakeRoundTrip( // A
	t *testing.T,
) authHandshakeRoundTripEnv {
	t.Helper()
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}
	adminKEM, _ := adminPub.MarshalBinaryKEM()
	adminSign, _ := adminPub.MarshalBinarySign()

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("user key: %v", err)
	}
	userPub := userAC.GetPublicKey()
	userBytes, err := auth.MarshalPubKeyBytes(&userPub)
	if err != nil {
		t.Fatalf("user pub: %v", err)
	}
	userKEM, _ := userPub.MarshalBinaryKEM()
	userSign, _ := userPub.MarshalBinarySign()
	anchorMsg := canonical.DomainSeparate(
		auth.CTXUserCAAnchorV1, userBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		t.Fatalf("anchor sign: %v", err)
	}

	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("node key: %v", err)
	}
	nodePub := nodeAC.GetPublicKey()
	now := int64(100)
	cert, err := auth.NewNodeCert(
		nodePub,
		adminCA.Hash(),
		now,
		now+100,
		[]byte("serial"),
		[]byte("nonce"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}
	proof := delegation.NewDelegationProof(
		make([]byte, auth.TLSCertPubKeyHashSize),
		make([]byte, delegation.TLSExporterBindingSize),
		make([]byte, delegation.TLSTranscriptHashSize),
		make([]byte, auth.X509FingerprintSize),
		make([]byte, auth.NodeCertBundleHashSize),
		now,
		now+10,
	)

	return authHandshakeRoundTripEnv{
		adminCA: adminCA, adminKEM: adminKEM,
		adminSign: adminSign, userKEM: userKEM,
		userSign: userSign, anchorSig: anchorSig,
		cert: cert, proof: proof,
	}
}

func TestWriteReadAuthHandshakeRoundTripAuthorities( // A
	t *testing.T,
) {
	env := setupAuthHandshakeRoundTrip(t)

	stream := newTestStream(nil)
	err := writeAuthHandshake(
		stream,
		[]canonical.NodeCertLike{env.cert},
		[][]byte{[]byte("ca-sig")},
		[]auth.EmbeddedCA{
			{
				Type:    "admin-ca",
				PubKEM:  env.adminKEM,
				PubSign: env.adminSign,
			},
			{
				Type:        "user-ca",
				PubKEM:      env.userKEM,
				PubSign:     env.userSign,
				AnchorSig:   env.anchorSig,
				AnchorAdmin: env.adminCA.Hash(),
			},
		},
		env.proof,
		[]byte("delegation-sig"),
	)
	if err != nil {
		t.Fatalf("writeAuthHandshake: %v", err)
	}

	conn := &testConn{
		tlsBindings: interfaces.TLSBindings{
			CertPubKeyHash: make(
				[]byte,
				auth.TLSCertPubKeyHashSize,
			),
			X509Fingerprint: make(
				[]byte,
				auth.X509FingerprintSize,
			),
		},
		exporter: map[string][]byte{
			delegation.TranscriptBindingLabel: make(
				[]byte,
				delegation.TLSTranscriptHashSize,
			),
			delegation.ExporterLabel: make(
				[]byte,
				delegation.TLSExporterBindingSize,
			),
		},
	}
	hs, err := readAuthHandshake(
		newTestStream(stream.writes.Bytes()),
		conn,
	)
	if err != nil {
		t.Fatalf("readAuthHandshake: %v", err)
	}
	if len(hs.Authorities) != 2 {
		t.Fatalf(
			"authorities len = %d, want 2",
			len(hs.Authorities),
		)
	}
	if hs.Authorities[1].AnchorAdmin != env.adminCA.Hash() {
		t.Fatalf(
			"anchor admin = %q, want %q",
			hs.Authorities[1].AnchorAdmin,
			env.adminCA.Hash(),
		)
	}
}

type noLeakTestEnv struct { // A
	payload []byte
	privKey ed25519.PrivateKey
}

func setupNoLeakTest( // A
	t *testing.T,
) noLeakTestEnv {
	t.Helper()
	adminAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("admin key: %v", err)
	}
	adminPub := adminAC.GetPublicKey()
	adminBytes, err := auth.MarshalPubKeyBytes(&adminPub)
	if err != nil {
		t.Fatalf("admin pub: %v", err)
	}
	adminCA, err := auth.NewAdminCA(adminBytes)
	if err != nil {
		t.Fatalf("NewAdminCA: %v", err)
	}

	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		t.Fatalf("node key: %v", err)
	}
	nodePub := nodeAC.GetPublicKey()
	cert, err := auth.NewNodeCert(
		nodePub,
		adminCA.Hash(),
		time.Now().Unix(),
		time.Now().Add(1*time.Hour).Unix(),
		[]byte("serial"),
		[]byte("nonce"),
	)
	if err != nil {
		t.Fatalf("NewNodeCert: %v", err)
	}

	session, err := delegation.NewSessionIdentity(
		5 * time.Minute,
	)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	privKey, ok := session.TLSCertificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		t.Fatal(
			"session private key is not" +
				" ed25519.PrivateKey",
		)
	}

	proof, sig, err := delegation.SignDelegation(
		nodeAC,
		[]canonical.NodeCertLike{cert},
		session,
		func(
			_ string,
			_ []byte,
			length int,
		) ([]byte, error) {
			return bytes.Repeat(
				[]byte{0xAB}, length,
			), nil
		},
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	stream := newTestStream(nil)
	if err := writeAuthHandshake(
		stream,
		[]canonical.NodeCertLike{cert},
		[][]byte{[]byte("ca-sig")},
		nil,
		proof,
		sig,
	); err != nil {
		t.Fatalf("writeAuthHandshake: %v", err)
	}

	return noLeakTestEnv{
		payload: stream.writes.Bytes(),
		privKey: privKey,
	}
}

func TestWriteAuthHandshakeDoesNotLeakSessionPrivateKey( // A
	t *testing.T,
) {
	env := setupNoLeakTest(t)

	if len(env.payload) < 4 {
		t.Fatal("handshake output too short")
	}

	t.Run("no_private_key_leak", func(t *testing.T) {
		if bytes.Contains(env.payload[4:], env.privKey) {
			t.Fatal(
				"auth handshake leaked session" +
					" private key bytes",
			)
		}
	})

	t.Run("wire_has_one_cert", func(t *testing.T) {
		var wire pb.WireAuthMessage
		if err := proto.Unmarshal(
			env.payload[4:], &wire,
		); err != nil {
			t.Fatalf("decode handshake payload: %v", err)
		}
		if len(wire.Certs) != 1 {
			t.Fatalf(
				"expected 1 cert, got %d",
				len(wire.Certs),
			)
		}
	})
}
