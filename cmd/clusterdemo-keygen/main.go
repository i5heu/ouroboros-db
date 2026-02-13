// GENERATED, NO HUMAN REVIEW, DO NOT RUN ON PRODUCTION SYSTEMS
package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

type nodeOutput struct { // A
	NodeCertB64 string `json:"nodeCertB64"`
	CASigB64    string `json:"caSigB64"`
	NodeKeyJSON string `json:"nodeKeyJSON"`
}

type keygenOutput struct { // A
	CAPubKem  string       `json:"caPubKem"`
	CAPubSign string       `json:"caPubSign"`
	Nodes     []nodeOutput `json:"nodes"`
}

func main() { // A
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(
			os.Stderr,
			"clusterdemo-keygen: %v\n",
			err,
		)
		os.Exit(1)
	}
}

func run() error { // A
	nodeCount := flag.Int(
		"nodes",
		3,
		"number of node key sets to generate",
	)
	flag.Parse()

	caSigner, err := keys.NewAsyncCrypt()
	if err != nil {
		return fmt.Errorf("generate CA keys: %w", err)
	}

	caPubVal := caSigner.GetPublicKey()
	caPub := &caPubVal

	caPubKem, err := caPub.ToBase64KEM()
	if err != nil {
		return fmt.Errorf("encode CA pub kem: %w", err)
	}

	caPubSign, err := caPub.ToBase64Sign()
	if err != nil {
		return fmt.Errorf("encode CA pub sign: %w", err)
	}

	adminCA, err := auth.NewAdminCA(caPub)
	if err != nil {
		return fmt.Errorf("create admin CA: %w", err)
	}

	out := keygenOutput{
		CAPubKem:  caPubKem,
		CAPubSign: caPubSign,
		Nodes:     make([]nodeOutput, 0, *nodeCount),
	}

	now := time.Now().UTC()

	for i := range *nodeCount {
		no, err := generateNode(
			caSigner,
			adminCA,
			now,
		)
		if err != nil {
			return fmt.Errorf(
				"generate node %d: %w",
				i,
				err,
			)
		}
		out.Nodes = append(out.Nodes, no)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func generateNode( // A
	caSigner *keys.AsyncCrypt,
	adminCA *auth.AdminCA,
	now time.Time,
) (nodeOutput, error) {
	nodeSigner, err := keys.NewAsyncCrypt()
	if err != nil {
		return nodeOutput{}, fmt.Errorf(
			"generate node keys: %w",
			err,
		)
	}

	nodePubVal := nodeSigner.GetPublicKey()
	nodePub := &nodePubVal

	var serial [16]byte
	if _, err := rand.Read(serial[:]); err != nil {
		return nodeOutput{}, fmt.Errorf(
			"generate serial: %w",
			err,
		)
	}

	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nodeOutput{}, fmt.Errorf(
			"generate nonce: %w",
			err,
		)
	}

	cert, err := auth.NewNodeCert(auth.NodeCertParams{
		NodePubKey:   nodePub,
		IssuerCAHash: adminCA.Hash(),
		ValidFrom:    now.Add(-time.Hour),
		ValidUntil:   now.Add(24 * time.Hour),
		Serial:       serial,
		RoleClaims:   auth.ScopeAdmin,
		CertNonce:    nonce,
	})
	if err != nil {
		return nodeOutput{}, fmt.Errorf(
			"create node cert: %w",
			err,
		)
	}

	caSig, err := auth.SignNodeCert(caSigner, cert)
	if err != nil {
		return nodeOutput{}, fmt.Errorf(
			"sign node cert: %w",
			err,
		)
	}

	certBytes, err := auth.MarshalNodeCert(cert)
	if err != nil {
		return nodeOutput{}, fmt.Errorf(
			"marshal node cert: %w",
			err,
		)
	}

	nodeKeyJSON, err := exportKeyJSON(nodeSigner)
	if err != nil {
		return nodeOutput{}, fmt.Errorf(
			"export node key: %w",
			err,
		)
	}

	return nodeOutput{
		NodeCertB64: base64.StdEncoding.EncodeToString(
			certBytes,
		),
		CASigB64: base64.StdEncoding.EncodeToString(caSig),
		NodeKeyJSON: nodeKeyJSON,
	}, nil
}

func exportKeyJSON( // A
	ac *keys.AsyncCrypt,
) (string, error) {
	tmpFile, err := os.CreateTemp("", "keygen-*.json")
	if err != nil {
		return "", fmt.Errorf(
			"create temp file: %w",
			err,
		)
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := ac.SaveToFile(tmpPath); err != nil {
		return "", fmt.Errorf("save key: %w", err)
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", fmt.Errorf(
			"read key file: %w",
			err,
		)
	}

	return string(data), nil
}
