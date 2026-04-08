// Package authfile reads and writes OuroborosDB
// credential files (.oukey and .oucert).
package authfile

import (
	"fmt"
	"os"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
	"github.com/i5heu/ouroboros-db/pkg/interfaces"
	pb "github.com/i5heu/ouroboros-db/proto/authfile"
	"google.golang.org/protobuf/proto"
)

// CAKeyFile is the protobuf-encoded .oukey format.
type CAKeyFile struct { // A
	Type               string
	KeyJSON            []byte
	AnchorSig          []byte
	AnchorAdmin        string
	AnchorAdminPubKEM  []byte
	AnchorAdminPubSign []byte
}

// EmbeddedCAFile stores public-only CA data that can
// be embedded inside a node cert for runtime trust
// bootstrapping without shipping CA private keys.
type EmbeddedCAFile struct { // A
	Type        string
	PubKEM      []byte
	PubSign     []byte
	AnchorSig   []byte
	AnchorAdmin string
}

// NodeCertFile is the protobuf-encoded .oucert
// format. It bundles everything a node needs to
// authenticate: its private key, the CA pubkey,
// and the CA signature over the NodeCert.
type NodeCertFile struct { // A
	Type         string
	KeyJSON      []byte
	CAPubKEM     []byte
	CAPubSign    []byte
	CASignature  []byte
	Authorities  []EmbeddedCAFile
	IssuerCAHash string
	ValidFrom    int64
	ValidUntil   int64
	Serial       []byte
	CertNonce    []byte
}

// MarshalKeyJSON serializes an AsyncCrypt keypair
// into the ouroboros-crypt JSON format.
func MarshalKeyJSON( // A
	ac *keys.AsyncCrypt,
) ([]byte, error) {
	tmp, err := os.CreateTemp("", "oukey-*.json")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	defer func() { _ = tmp.Close() }()
	defer func() { _ = os.Remove(name) }()
	if err := ac.SaveToFile(name); err != nil {
		return nil, err
	}
	return os.ReadFile(name) //#nosec G304 // safe: temp file we just wrote
}

// LoadKeyFromJSON reconstructs an AsyncCrypt
// keypair from the ouroboros-crypt JSON format.
func LoadKeyFromJSON(data []byte) ( // A
	*keys.AsyncCrypt, error,
) {
	tmp, err := os.CreateTemp("", "oukey-*.json")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	defer func() { _ = tmp.Close() }()
	defer func() { _ = os.Remove(name) }()
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return nil, err
	}
	return keys.NewCryptFromFile(name)
}

// WriteFile writes data to path with 0600 perms.
func WriteFile(path string, data []byte) error { // A
	return os.WriteFile(path, data, 0o600)
}

// MarshalCAKey encodes a CAKeyFile to protobuf bytes.
func MarshalCAKey(f *CAKeyFile) ([]byte, error) { // A
	msg := &pb.CAKeyFile{
		Type:               f.Type,
		KeyJson:            f.KeyJSON,
		AnchorSig:          f.AnchorSig,
		AnchorAdmin:        f.AnchorAdmin,
		AnchorAdminPubKem:  f.AnchorAdminPubKEM,
		AnchorAdminPubSign: f.AnchorAdminPubSign,
	}
	return proto.Marshal(msg)
}

// MarshalNodeCert encodes a NodeCertFile to protobuf.
func MarshalNodeCert( // A
	f *NodeCertFile,
) ([]byte, error) {
	authorities := make(
		[]*pb.EmbeddedCAFile, len(f.Authorities),
	)
	for i, a := range f.Authorities {
		authorities[i] = &pb.EmbeddedCAFile{
			Type:        a.Type,
			PubKem:      a.PubKEM,
			PubSign:     a.PubSign,
			AnchorSig:   a.AnchorSig,
			AnchorAdmin: a.AnchorAdmin,
		}
	}
	msg := &pb.NodeCertFile{
		Type:         f.Type,
		KeyJson:      f.KeyJSON,
		CaPubKem:     f.CAPubKEM,
		CaPubSign:    f.CAPubSign,
		CaSignature:  f.CASignature,
		Authorities:  authorities,
		IssuerCaHash: f.IssuerCAHash,
		ValidFrom:    f.ValidFrom,
		ValidUntil:   f.ValidUntil,
		Serial:       f.Serial,
		CertNonce:    f.CertNonce,
	}
	return proto.Marshal(msg)
}

// UnmarshalCAKey decodes protobuf bytes into a
// CAKeyFile.
func UnmarshalCAKey(data []byte) (*CAKeyFile, error) { // A
	var msg pb.CAKeyFile
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return &CAKeyFile{
		Type:               msg.Type,
		KeyJSON:            msg.KeyJson,
		AnchorSig:          msg.AnchorSig,
		AnchorAdmin:        msg.AnchorAdmin,
		AnchorAdminPubKEM:  msg.AnchorAdminPubKem,
		AnchorAdminPubSign: msg.AnchorAdminPubSign,
	}, nil
}

// UnmarshalNodeCert decodes protobuf bytes into a
// NodeCertFile.
func UnmarshalNodeCert( // A
	data []byte,
) (*NodeCertFile, error) {
	var msg pb.NodeCertFile
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	authorities := make(
		[]EmbeddedCAFile, len(msg.Authorities),
	)
	for i, a := range msg.Authorities {
		authorities[i] = EmbeddedCAFile{
			Type:        a.Type,
			PubKEM:      a.PubKem,
			PubSign:     a.PubSign,
			AnchorSig:   a.AnchorSig,
			AnchorAdmin: a.AnchorAdmin,
		}
	}
	return &NodeCertFile{
		Type:         msg.Type,
		KeyJSON:      msg.KeyJson,
		CAPubKEM:     msg.CaPubKem,
		CAPubSign:    msg.CaPubSign,
		CASignature:  msg.CaSignature,
		Authorities:  authorities,
		IssuerCAHash: msg.IssuerCaHash,
		ValidFrom:    msg.ValidFrom,
		ValidUntil:   msg.ValidUntil,
		Serial:       msg.Serial,
		CertNonce:    msg.CertNonce,
	}, nil
}

// ReadCAKey reads and decodes a .oukey file,
// returning the keypair and parsed metadata.
func ReadCAKey(path string) ( // A
	*keys.AsyncCrypt, *CAKeyFile, error,
) {
	//#nosec G304 // safe: path from caller for credential loading
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"read %s: %w", path, err,
		)
	}
	f, err := UnmarshalCAKey(data)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"decode %s: %w", path, err,
		)
	}
	if f.Type != "admin-ca" && f.Type != "user-ca" {
		return nil, nil, fmt.Errorf(
			"%s: not a CA key (type=%q)",
			path, f.Type,
		)
	}
	ac, err := LoadKeyFromJSON(f.KeyJSON)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"load key: %w", err,
		)
	}
	return ac, f, nil
}

// ReadNodeCert reads and decodes a .oucert file,
// returning the keypair and parsed metadata.
func ReadNodeCert(path string) ( // A
	*keys.AsyncCrypt, *NodeCertFile, error,
) {
	data, err := os.ReadFile(
		path,
	) //#nosec G304 // safe: path provided by caller for credential loading
	if err != nil {
		return nil, nil, fmt.Errorf(
			"read %s: %w", path, err,
		)
	}
	f, err := UnmarshalNodeCert(data)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"decode %s: %w", path, err,
		)
	}
	if f.Type != "node-cert" {
		return nil, nil, fmt.Errorf(
			"%s: not a node cert (type=%q)",
			path, f.Type,
		)
	}
	ac, err := LoadKeyFromJSON(f.KeyJSON)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"load key: %w", err,
		)
	}
	return ac, f, nil
}

// NodeCertToIdentity converts a loaded NodeCertFile
// into an certpkg.NodeIdentity ready for connections.
func NodeCertToIdentity( // A
	ac *keys.AsyncCrypt,
	f *NodeCertFile,
) (*certpkg.NodeIdentity, error) {
	pub := ac.GetPublicKey()
	cert, err := certpkg.NewNodeCert(
		pub,
		f.IssuerCAHash,
		f.ValidFrom,
		f.ValidUntil,
		f.Serial,
		f.CertNonce,
	)
	if err != nil {
		return nil, fmt.Errorf("node cert: %w", err)
	}
	certs := []canonical.NodeCertLike{cert}
	sigs := [][]byte{f.CASignature}
	return certpkg.NewNodeIdentity(
		ac,
		certs,
		sigs,
		toAuthEmbeddedCAs(f.Authorities),
	)
}

// BuildEmbeddedTrustChain derives the public trust
// chain to embed in a node cert from the issuing CA
// material used at signing time.
func BuildEmbeddedTrustChain( // A
	ac *keys.AsyncCrypt,
	f *CAKeyFile,
) ([]EmbeddedCAFile, error) {
	pub := ac.GetPublicKey()
	pubKEM, err := pub.MarshalBinaryKEM()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal CA KEM pubkey: %w", err,
		)
	}
	pubSign, err := pub.MarshalBinarySign()
	if err != nil {
		return nil, fmt.Errorf(
			"marshal CA sign pubkey: %w", err,
		)
	}
	switch f.Type {
	case "admin-ca":
		return []EmbeddedCAFile{{
			Type:    "admin-ca",
			PubKEM:  pubKEM,
			PubSign: pubSign,
		}}, nil
	case "user-ca":
		if len(f.AnchorAdminPubKEM) == 0 ||
			len(f.AnchorAdminPubSign) == 0 {
			return nil, fmt.Errorf(
				"user CA is missing anchor admin" +
					" public key",
			)
		}
		return []EmbeddedCAFile{
			{
				Type:    "admin-ca",
				PubKEM:  append([]byte(nil), f.AnchorAdminPubKEM...),
				PubSign: append([]byte(nil), f.AnchorAdminPubSign...),
			},
			{
				Type:        "user-ca",
				PubKEM:      pubKEM,
				PubSign:     pubSign,
				AnchorSig:   append([]byte(nil), f.AnchorSig...),
				AnchorAdmin: f.AnchorAdmin,
			},
		}, nil
	default:
		return nil, fmt.Errorf(
			"unsupported CA key type %q", f.Type,
		)
	}
}

// AddEmbeddedTrust populates a trust store from the
// public chain embedded in a node cert. Legacy node
// certs without Authorities fall back to trusting the
// single issuer pubkey stored at the top level.
func AddEmbeddedTrust( // A
	store interfaces.TrustStore,
	f *NodeCertFile,
) error {
	authorities := f.Authorities
	if len(authorities) == 0 {
		authorities = []EmbeddedCAFile{{
			Type: "admin-ca",
			PubKEM: append(
				[]byte(nil), f.CAPubKEM...,
			),
			PubSign: append(
				[]byte(nil), f.CAPubSign...,
			),
		}}
	}
	for _, authority := range authorities {
		if authority.Type != "admin-ca" {
			continue
		}
		pubBytes, err := embeddedCAPubKeyBytes(authority)
		if err != nil {
			return err
		}
		if err := store.AddAdminPubKey(pubBytes); err != nil {
			return err
		}
	}
	for _, authority := range authorities {
		if authority.Type != "user-ca" {
			continue
		}
		pubBytes, err := embeddedCAPubKeyBytes(authority)
		if err != nil {
			return err
		}
		if err := store.AddUserPubKey(
			pubBytes,
			authority.AnchorSig,
			authority.AnchorAdmin,
		); err != nil {
			return err
		}
	}
	return nil
}

func embeddedCAPubKeyBytes( // A
	authority EmbeddedCAFile,
) ([]byte, error) {
	pub, err := keys.NewPublicKeyFromBinary(
		authority.PubKEM,
		authority.PubSign,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rebuild embedded CA pubkey: %w", err,
		)
	}
	pubBytes, err := auth.MarshalPubKeyBytes(pub)
	if err != nil {
		return nil, fmt.Errorf(
			"marshal embedded CA pubkey: %w", err,
		)
	}
	return pubBytes, nil
}

func toAuthEmbeddedCAs( // A
	authorities []EmbeddedCAFile,
) []auth.EmbeddedCA {
	out := make([]auth.EmbeddedCA, len(authorities))
	for index, authority := range authorities {
		out[index] = auth.EmbeddedCA{
			Type:        authority.Type,
			PubKEM:      append([]byte(nil), authority.PubKEM...),
			PubSign:     append([]byte(nil), authority.PubSign...),
			AnchorSig:   append([]byte(nil), authority.AnchorSig...),
			AnchorAdmin: authority.AnchorAdmin,
		}
	}
	return out
}
