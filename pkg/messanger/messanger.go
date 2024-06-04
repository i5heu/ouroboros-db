package messanger

import (
	"crypto"

	"github.com/i5heu/ouroboros-db/pkg/types"
)

type Identity struct {
	Hash           types.Hash
	PublicKey      *crypto.PublicKey
	RootSignature1 []byte // ed25519
	RootSignature2 []byte // dilithium/mode5 - post quantum
}

type RootCertificate struct {
	RootPublicKey *crypto.PublicKey
}

type selfIdentity struct {
	privatekey *crypto.PrivateKey
}

type Message struct {
}
