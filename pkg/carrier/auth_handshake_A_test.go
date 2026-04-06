package carrier

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
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

func TestReadAuthHandshakeRejectsMalformedAuthoritiesCBOR( // A
	t *testing.T,
) {
	payload, err := cbor.Marshal(map[int]any{
		1: []map[int]any{},
		2: [][]byte{},
		3: []map[int]any{{1: "admin-ca", 2: 7}},
		4: map[int]any{},
		5: []byte("sig"),
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	frame := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	conn := &testConn{
		tlsBindings: interfaces.TLSBindings{},
		exporter: map[string][]byte{
			auth.TranscriptBindingLabel: make([]byte, auth.TLSTranscriptHashSize),
			auth.ExporterLabel:          make([]byte, auth.TLSExporterBindingSize),
		},
	}
	_, err = readAuthHandshake(newTestStream(frame), conn)
	if err == nil {
		t.Fatal("expected malformed authority payload to fail")
	}
}

func TestWriteReadAuthHandshakeRoundTripAuthorities( // A
	t *testing.T,
) {
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
	anchorMsg := auth.DomainSeparate(auth.CTXUserCAAnchorV1, userBytes)
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
	proof := auth.NewDelegationProof(
		make([]byte, auth.TLSCertPubKeyHashSize),
		make([]byte, auth.TLSExporterBindingSize),
		make([]byte, auth.TLSTranscriptHashSize),
		make([]byte, auth.X509FingerprintSize),
		make([]byte, auth.NodeCertBundleHashSize),
		now,
		now+10,
	)

	stream := newTestStream(nil)
	err = writeAuthHandshake(
		stream,
		[]auth.NodeCertLike{cert},
		[][]byte{[]byte("ca-sig")},
		[]auth.EmbeddedCA{
			{Type: "admin-ca", PubKEM: adminKEM, PubSign: adminSign},
			{
				Type:        "user-ca",
				PubKEM:      userKEM,
				PubSign:     userSign,
				AnchorSig:   anchorSig,
				AnchorAdmin: adminCA.Hash(),
			},
		},
		proof,
		[]byte("delegation-sig"),
	)
	if err != nil {
		t.Fatalf("writeAuthHandshake: %v", err)
	}

	conn := &testConn{
		tlsBindings: interfaces.TLSBindings{
			CertPubKeyHash:  make([]byte, auth.TLSCertPubKeyHashSize),
			X509Fingerprint: make([]byte, auth.X509FingerprintSize),
		},
		exporter: map[string][]byte{
			auth.TranscriptBindingLabel: make([]byte, auth.TLSTranscriptHashSize),
			auth.ExporterLabel:          make([]byte, auth.TLSExporterBindingSize),
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
		t.Fatalf("authorities len = %d, want 2", len(hs.Authorities))
	}
	if hs.Authorities[1].AnchorAdmin != adminCA.Hash() {
		t.Fatalf(
			"anchor admin = %q, want %q",
			hs.Authorities[1].AnchorAdmin,
			adminCA.Hash(),
		)
	}
}

func TestWriteAuthHandshakeDoesNotLeakSessionPrivateKey( // A
	t *testing.T,
) {
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

	session, err := auth.NewSessionIdentity(5 * time.Minute)
	if err != nil {
		t.Fatalf("NewSessionIdentity: %v", err)
	}

	privKey, ok := session.TLSCertificate.PrivateKey.(ed25519.PrivateKey)
	if !ok {
		t.Fatal("session private key is not ed25519.PrivateKey")
	}

	proof, sig, err := auth.SignDelegation(
		nodeAC,
		[]auth.NodeCertLike{cert},
		session,
		func(label string, _ []byte, length int) ([]byte, error) {
			return bytes.Repeat([]byte{0xAB}, length), nil
		},
	)
	if err != nil {
		t.Fatalf("SignDelegation: %v", err)
	}

	stream := newTestStream(nil)
	if err := writeAuthHandshake(
		stream,
		[]auth.NodeCertLike{cert},
		[][]byte{[]byte("ca-sig")},
		nil,
		proof,
		sig,
	); err != nil {
		t.Fatalf("writeAuthHandshake: %v", err)
	}

	payload := stream.writes.Bytes()
	if len(payload) < 4 {
		t.Fatal("handshake output too short")
	}
	if bytes.Contains(payload[4:], privKey) {
		t.Fatal("auth handshake leaked session private key bytes")
	}

	var wire wireAuthMessage
	if err := cbor.Unmarshal(payload[4:], &wire); err != nil {
		t.Fatalf("decode handshake payload: %v", err)
	}
	if len(wire.Certs) != 1 {
		t.Fatalf("expected 1 cert, got %d", len(wire.Certs))
	}
}
