// Package authfile reads and writes OuroborosDB
// credential files (.oukey and .oucert).
package authfile

import (
	"fmt"
	"os"

	"github.com/fxamacker/cbor/v2"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/pkg/auth"
)

// CAKeyFile is the CBOR-encoded .oukey format.
type CAKeyFile struct { // A
Type        string `cbor:"type"`
KeyJSON     []byte `cbor:"keyJSON"`
AnchorSig   []byte `cbor:"anchorSig,omitempty"`
AnchorAdmin string `cbor:"anchorAdmin,omitempty"`
}

// NodeCertFile is the CBOR-encoded .oucert format.
// It bundles everything a node needs to authenticate:
// its private key, the CA pubkey, and the CA signature
// over the NodeCert.
type NodeCertFile struct { // A
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
tmp.Close()
defer os.Remove(name)
if err := ac.SaveToFile(name); err != nil {
return nil, err
}
return os.ReadFile(name)
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
tmp.Close()
defer os.Remove(name)
if err := os.WriteFile(name, data, 0o600); err != nil {
return nil, err
}
return keys.NewCryptFromFile(name)
}

// WriteFile writes data to path with 0600 perms.
func WriteFile(path string, data []byte) error { // A
return os.WriteFile(path, data, 0o600)
}

// MarshalCAKey encodes a CAKeyFile to CBOR bytes.
func MarshalCAKey(f *CAKeyFile) ([]byte, error) { // A
return cborEnc.Marshal(f)
}

// MarshalNodeCert encodes a NodeCertFile to CBOR.
func MarshalNodeCert( // A
f *NodeCertFile,
) ([]byte, error) {
return cborEnc.Marshal(f)
}

// ReadCAKey reads and decodes a .oukey file,
// returning the keypair and parsed metadata.
func ReadCAKey(path string) ( // A
*keys.AsyncCrypt, *CAKeyFile, error,
) {
data, err := os.ReadFile(path)
if err != nil {
return nil, nil, fmt.Errorf(
"read %s: %w", path, err,
)
}
var f CAKeyFile
if err := cbor.Unmarshal(data, &f); err != nil {
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
return ac, &f, nil
}

// ReadNodeCert reads and decodes a .oucert file,
// returning the keypair and parsed metadata.
func ReadNodeCert(path string) ( // A
*keys.AsyncCrypt, *NodeCertFile, error,
) {
data, err := os.ReadFile(path)
if err != nil {
return nil, nil, fmt.Errorf(
"read %s: %w", path, err,
)
}
var f NodeCertFile
if err := cbor.Unmarshal(data, &f); err != nil {
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
return ac, &f, nil
}

// NodeCertToIdentity converts a loaded NodeCertFile
// into an auth.NodeIdentity ready for connections.
func NodeCertToIdentity( // A
ac *keys.AsyncCrypt,
f *NodeCertFile,
) (*auth.NodeIdentity, error) {
pub := ac.GetPublicKey()
cert, err := auth.NewNodeCert(
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
certs := []auth.NodeCertLike{cert}
sigs := [][]byte{f.CASignature}
return auth.NewNodeIdentity(ac, certs, sigs)
}
