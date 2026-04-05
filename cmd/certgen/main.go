// certgen generates and manages OuroborosDB
// certificate authorities and node credentials.
//
// Usage:
//
//	certgen admin-ca  -out admin.oukey
//	certgen user-ca   -admin-key admin.oukey -out user.oukey
//	certgen sign-node -ca-key <ca>.oukey     -out node.oucert
//	certgen show      -file <file>
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"os"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

// caKeyFile is the CBOR-encoded .oukey format.
// KeyJSON holds the ouroboros-crypt JSON key blob.
type caKeyFile struct { // A
	Type        string `cbor:"type"`
	KeyJSON     []byte `cbor:"keyJSON"`
	AnchorSig   []byte `cbor:"anchorSig,omitempty"`
	AnchorAdmin string `cbor:"anchorAdmin,omitempty"`
}

// nodeCertFile is the CBOR-encoded .oucert format.
// It bundles everything a node needs to authenticate:
// its private key, the CA pubkey, and the CA signature
// over the NodeCert.
type nodeCertFile struct { // A
	Type         string `cbor:"type"`
	KeyJSON      []byte `cbor:"keyJSON"`
	CAPubKEM     []byte `cbor:"caPubKEM"`
	CAPubSign    []byte `cbor:"caPubSign"`
	CASignature  []byte `cbor:"caSignature"`
	IssuerCAHash string `cbor:"issuerCAHash"`
	ValidFrom    int64  `cbor:"validFrom"`
	ValidUntil   int64  `cbor:"validUntil"`
	Serial       []byte `cbor:"serial"`
	CertNonce    []byte `cbor:"certNonce"`
}

var cborEnc cbor.EncMode // A

func init() { // A
	opts := cbor.CanonicalEncOptions()
	var err error
	cborEnc, err = opts.EncMode()
	if err != nil {
		panic(err)
	}
}

func main() { // A
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "admin-ca":
		err = cmdAdminCA(os.Args[2:])
	case "user-ca":
		err = cmdUserCA(os.Args[2:])
	case "sign-node":
		err = cmdSignNode(os.Args[2:])
	case "show":
		err = cmdShow(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() { // A
	fmt.Fprintf(os.Stderr, `Usage:
  certgen admin-ca  -out <file.oukey>
  certgen user-ca   -admin-key <admin.oukey> -out <file.oukey>
  certgen sign-node -ca-key <ca.oukey> -out <node.oucert>
                    [-validity <duration>]
  certgen show      -file <file>
`)
	os.Exit(2)
}

func parseFlag( // A
	args []string,
	name string,
) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func marshalKeyJSON( // A
	ac *keys.AsyncCrypt,
) ([]byte, error) {
	tmp, err := os.CreateTemp("", "oukey-*.json")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	if err := ac.SaveToFile(name); err != nil {
		return nil, err
	}
	return os.ReadFile(name)
}

func loadKeyFromJSON(data []byte) ( // A
	*keys.AsyncCrypt, error,
) {
	tmp, err := os.CreateTemp("", "oukey-*.json")
	if err != nil {
		return nil, err
	}
	name := tmp.Name()
	tmp.Close()
	defer os.Remove(name)
	if err := os.WriteFile(name, data, 0o600); err != nil {
		return nil, err
	}
	return keys.NewCryptFromFile(name)
}

func writeFile(path string, data []byte) error { // A
	return os.WriteFile(path, data, 0o600)
}

func cmdAdminCA(args []string) error { // A
	out, ok := parseFlag(args, "-out")
	if !ok {
		return fmt.Errorf("missing -out flag")
	}
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		return fmt.Errorf("key generation: %w", err)
	}
	keyJSON, err := marshalKeyJSON(ac)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	f := caKeyFile{
		Type:    "admin-ca",
		KeyJSON: keyJSON,
	}
	data, err := cborEnc.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := writeFile(out, data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	pub := ac.GetPublicKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		return fmt.Errorf("pubkey hash: %w", err)
	}
	adminCA, err := auth.NewAdminCA(pubBytes)
	if err != nil {
		return fmt.Errorf("admin CA: %w", err)
	}
	fmt.Printf("admin CA created: %s\n", out)
	fmt.Printf("  hash: %s\n", adminCA.Hash())
	return nil
}

func loadCAKey(path string) ( // A
	*keys.AsyncCrypt, *caKeyFile, error,
) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"read %s: %w", path, err,
		)
	}
	var f caKeyFile
	if err := cbor.Unmarshal(data, &f); err != nil {
		return nil, nil, fmt.Errorf(
			"decode %s: %w", path, err,
		)
	}
	if f.Type != "admin-ca" && f.Type != "user-ca" {
		return nil, nil, fmt.Errorf(
			"%s: not a CA key (type=%q)", path, f.Type,
		)
	}
	ac, err := loadKeyFromJSON(f.KeyJSON)
	if err != nil {
		return nil, nil, fmt.Errorf(
			"load key: %w", err,
		)
	}
	return ac, &f, nil
}

func cmdUserCA(args []string) error { // A
	adminPath, ok := parseFlag(args, "-admin-key")
	if !ok {
		return fmt.Errorf("missing -admin-key flag")
	}
	out, ok := parseFlag(args, "-out")
	if !ok {
		return fmt.Errorf("missing -out flag")
	}
	adminAC, adminFile, err := loadCAKey(adminPath)
	if err != nil {
		return err
	}
	if adminFile.Type != "admin-ca" {
		return fmt.Errorf(
			"%s is %s, need admin-ca",
			adminPath, adminFile.Type,
		)
	}
	adminPub := adminAC.GetPublicKey()
	adminPubBytes, err := auth.MarshalPubKeyBytes(
		&adminPub,
	)
	if err != nil {
		return fmt.Errorf("admin pubkey: %w", err)
	}
	adminCA, err := auth.NewAdminCA(adminPubBytes)
	if err != nil {
		return fmt.Errorf("admin CA: %w", err)
	}

	userAC, err := keys.NewAsyncCrypt()
	if err != nil {
		return fmt.Errorf("key generation: %w", err)
	}
	userPub := userAC.GetPublicKey()
	userPubBytes, err := auth.MarshalPubKeyBytes(
		&userPub,
	)
	if err != nil {
		return fmt.Errorf("user pubkey: %w", err)
	}

	// Sign anchor: admin signs user CA's pubkey.
	anchorMsg := auth.DomainSeparate(
		auth.CTXUserCAAnchorV1, userPubBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		return fmt.Errorf("anchor sign: %w", err)
	}

	keyJSON, err := marshalKeyJSON(userAC)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	f := caKeyFile{
		Type:        "user-ca",
		KeyJSON:     keyJSON,
		AnchorSig:   anchorSig,
		AnchorAdmin: adminCA.Hash(),
	}
	data, err := cborEnc.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := writeFile(out, data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	fmt.Printf("user CA created: %s\n", out)
	fmt.Printf("  anchored to admin: %s\n",
		adminCA.Hash(),
	)
	return nil
}

func cmdSignNode(args []string) error { // A
	caPath, ok := parseFlag(args, "-ca-key")
	if !ok {
		return fmt.Errorf("missing -ca-key flag")
	}
	out, ok := parseFlag(args, "-out")
	if !ok {
		return fmt.Errorf("missing -out flag")
	}
	validityStr, _ := parseFlag(args, "-validity")
	validity := 365 * 24 * time.Hour
	if validityStr != "" {
		v, err := time.ParseDuration(validityStr)
		if err != nil {
			return fmt.Errorf(
				"invalid -validity: %w", err,
			)
		}
		validity = v
	}

	caAC, _, err := loadCAKey(caPath)
	if err != nil {
		return err
	}
	caPub := caAC.GetPublicKey()
	caPubBytes, err := auth.MarshalPubKeyBytes(&caPub)
	if err != nil {
		return fmt.Errorf("ca pubkey: %w", err)
	}
	caPubKEM, err := caPub.MarshalBinaryKEM()
	if err != nil {
		return fmt.Errorf("ca pubkey KEM: %w", err)
	}
	caPubSign, err := caPub.MarshalBinarySign()
	if err != nil {
		return fmt.Errorf("ca pubkey Sign: %w", err)
	}

	// Derive CA hash for the cert's IssuerCAHash.
	h := sha256.Sum256(caPubBytes[auth.KEMPublicKeySize:])
	caHash := fmt.Sprintf("%x", h)

	// Generate the node keypair.
	nodeAC, err := keys.NewAsyncCrypt()
	if err != nil {
		return fmt.Errorf("node key gen: %w", err)
	}
	nodePub := nodeAC.GetPublicKey()

	now := time.Now()
	serial := make([]byte, 16)
	if _, err := rand.Read(serial); err != nil {
		return fmt.Errorf("serial: %w", err)
	}
	certNonce := make([]byte, 16)
	if _, err := rand.Read(certNonce); err != nil {
		return fmt.Errorf("nonce: %w", err)
	}

	cert, err := auth.NewNodeCert(
		nodePub,
		caHash,
		now.Unix(),
		now.Add(validity).Unix(),
		serial,
		certNonce,
	)
	if err != nil {
		return fmt.Errorf("node cert: %w", err)
	}

	// Sign the canonical NodeCert.
	canonical, err := auth.CanonicalNodeCert(cert)
	if err != nil {
		return fmt.Errorf("canonical cert: %w", err)
	}
	msg := auth.DomainSeparate(
		auth.CTXNodeAdmissionV1, canonical,
	)
	caSig, err := caAC.Sign(msg)
	if err != nil {
		return fmt.Errorf("sign cert: %w", err)
	}

	// Verify the signature round-trips.
	ca, err := auth.NewAdminCA(caPubBytes)
	if err != nil {
		return fmt.Errorf("verify CA: %w", err)
	}
	if _, err := ca.VerifyNodeCert(cert, caSig); err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}

	keyJSON, err := marshalKeyJSON(nodeAC)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	f := nodeCertFile{
		Type:         "node-cert",
		KeyJSON:      keyJSON,
		CAPubKEM:     caPubKEM,
		CAPubSign:    caPubSign,
		CASignature:  caSig,
		IssuerCAHash: caHash,
		ValidFrom:    now.Unix(),
		ValidUntil:   now.Add(validity).Unix(),
		Serial:       serial,
		CertNonce:    certNonce,
	}
	data, err := cborEnc.Marshal(f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := writeFile(out, data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	nodeID := cert.NodeID()
	fmt.Printf("node cert created: %s\n", out)
	fmt.Printf("  node ID:   %x\n", nodeID[:])
	fmt.Printf("  issuer CA: %s\n", caHash)
	fmt.Printf("  valid:     %s to %s\n",
		now.Format(time.DateOnly),
		now.Add(validity).Format(time.DateOnly),
	)
	return nil
}

func cmdShow(args []string) error { // A
	path, ok := parseFlag(args, "-file")
	if !ok {
		return fmt.Errorf("missing -file flag")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	// Try caKeyFile first.
	var ca caKeyFile
	if err := cbor.Unmarshal(data, &ca); err == nil {
		if ca.Type == "admin-ca" || ca.Type == "user-ca" {
			return showCA(&ca)
		}
	}

	// Try nodeCertFile.
	var nc nodeCertFile
	if err := cbor.Unmarshal(data, &nc); err == nil {
		if nc.Type == "node-cert" {
			return showNode(&nc)
		}
	}

	return fmt.Errorf("unrecognized file format")
}

func showCA(f *caKeyFile) error { // A
	ac, err := loadKeyFromJSON(f.KeyJSON)
	if err != nil {
		return fmt.Errorf("load key: %w", err)
	}
	pub := ac.GetPublicKey()
	pubBytes, err := auth.MarshalPubKeyBytes(&pub)
	if err != nil {
		return fmt.Errorf("marshal pub: %w", err)
	}
	ca, err := auth.NewAdminCA(pubBytes)
	if err != nil {
		return fmt.Errorf("admin CA: %w", err)
	}
	fmt.Printf("type:    %s\n", f.Type)
	fmt.Printf("hash:    %s\n", ca.Hash())
	if f.Type == "user-ca" {
		fmt.Printf("anchor:  %s\n", f.AnchorAdmin)
	}
	return nil
}

func showNode(f *nodeCertFile) error { // A
	ac, err := loadKeyFromJSON(f.KeyJSON)
	if err != nil {
		return fmt.Errorf("load key: %w", err)
	}
	pub := ac.GetPublicKey()
	nodeID, err := pub.NodeID()
	if err != nil {
		return fmt.Errorf("node ID: %w", err)
	}
	fmt.Printf("type:      node-cert\n")
	fmt.Printf("node ID:   %x\n", nodeID[:])
	fmt.Printf("issuer CA: %s\n", f.IssuerCAHash)
	fmt.Printf("valid:     %s to %s\n",
		time.Unix(f.ValidFrom, 0).Format(time.DateOnly),
		time.Unix(f.ValidUntil, 0).Format(time.DateOnly),
	)
	return nil
}
