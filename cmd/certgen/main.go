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
	"github.com/i5heu/ouroboros-db/pkg/authfile"
)

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

func cmdAdminCA(args []string) error { // A
	out, ok := parseFlag(args, "-out")
	if !ok {
		return fmt.Errorf("missing -out flag")
	}
	ac, err := keys.NewAsyncCrypt()
	if err != nil {
		return fmt.Errorf("key generation: %w", err)
	}
	keyJSON, err := authfile.MarshalKeyJSON(ac)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	f := authfile.CAKeyFile{
		Type:    "admin-ca",
		KeyJSON: keyJSON,
	}
	data, err := authfile.MarshalCAKey(&f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := authfile.WriteFile(out, data); err != nil {
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

func cmdUserCA(args []string) error { // A
	adminPath, ok := parseFlag(args, "-admin-key")
	if !ok {
		return fmt.Errorf("missing -admin-key flag")
	}
	out, ok := parseFlag(args, "-out")
	if !ok {
		return fmt.Errorf("missing -out flag")
	}
	adminAC, adminFile, err := authfile.ReadCAKey(
		adminPath,
	)
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

	anchorMsg := auth.DomainSeparate(
		auth.CTXUserCAAnchorV1, userPubBytes,
	)
	anchorSig, err := adminAC.Sign(anchorMsg)
	if err != nil {
		return fmt.Errorf("anchor sign: %w", err)
	}

	keyJSON, err := authfile.MarshalKeyJSON(userAC)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	adminPubKEM, err := adminPub.MarshalBinaryKEM()
	if err != nil {
		return fmt.Errorf("admin pubkey KEM: %w", err)
	}
	adminPubSign, err := adminPub.MarshalBinarySign()
	if err != nil {
		return fmt.Errorf("admin pubkey Sign: %w", err)
	}
	f := authfile.CAKeyFile{
		Type:               "user-ca",
		KeyJSON:            keyJSON,
		AnchorSig:          anchorSig,
		AnchorAdmin:        adminCA.Hash(),
		AnchorAdminPubKEM:  adminPubKEM,
		AnchorAdminPubSign: adminPubSign,
	}
	data, err := authfile.MarshalCAKey(&f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := authfile.WriteFile(out, data); err != nil {
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

	caAC, caFile, err := authfile.ReadCAKey(caPath)
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

	h := sha256.Sum256(
		caPubBytes[auth.KEMPublicKeySize:],
	)
	caHash := fmt.Sprintf("%x", h)

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

	ca, err := auth.NewAdminCA(caPubBytes)
	if err != nil {
		return fmt.Errorf("verify CA: %w", err)
	}
	if _, err := ca.VerifyNodeCert(cert, caSig); err != nil {
		return fmt.Errorf("verify failed: %w", err)
	}

	keyJSON, err := authfile.MarshalKeyJSON(nodeAC)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	authorities, err := authfile.BuildEmbeddedTrustChain(
		caAC,
		caFile,
	)
	if err != nil {
		return fmt.Errorf("embedded trust chain: %w", err)
	}
	f := authfile.NodeCertFile{
		Type:         "node-cert",
		KeyJSON:      keyJSON,
		CAPubKEM:     caPubKEM,
		CAPubSign:    caPubSign,
		CASignature:  caSig,
		Authorities:  authorities,
		IssuerCAHash: caHash,
		ValidFrom:    now.Unix(),
		ValidUntil:   now.Add(validity).Unix(),
		Serial:       serial,
		CertNonce:    certNonce,
	}
	data, err := authfile.MarshalNodeCert(&f)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	if err := authfile.WriteFile(out, data); err != nil {
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

	var ca authfile.CAKeyFile
	if err := cbor.Unmarshal(data, &ca); err == nil {
		if ca.Type == "admin-ca" ||
			ca.Type == "user-ca" {
			return showCA(&ca)
		}
	}

	var nc authfile.NodeCertFile
	if err := cbor.Unmarshal(data, &nc); err == nil {
		if nc.Type == "node-cert" {
			return showNode(&nc)
		}
	}

	return fmt.Errorf("unrecognized file format")
}

func showCA(f *authfile.CAKeyFile) error { // A
	ac, err := authfile.LoadKeyFromJSON(f.KeyJSON)
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

func showNode( // A
	f *authfile.NodeCertFile,
) error {
	ac, err := authfile.LoadKeyFromJSON(f.KeyJSON)
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
	fmt.Printf("authorities: %d\n", len(f.Authorities))
	fmt.Printf("valid:     %s to %s\n",
		time.Unix(f.ValidFrom, 0).Format(
			time.DateOnly,
		),
		time.Unix(f.ValidUntil, 0).Format(
			time.DateOnly,
		),
	)
	return nil
}
