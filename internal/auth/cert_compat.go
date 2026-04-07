package auth

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	"github.com/i5heu/ouroboros-db/internal/auth/canonical"
	certpkg "github.com/i5heu/ouroboros-db/internal/auth/cert"
)

const DefaultCertVersion = certpkg.DefaultCertVersion // A

type EmbeddedCA = certpkg.EmbeddedCA // A

type NodeCertImpl = certpkg.NodeCertImpl // A

type NodeIdentity = certpkg.NodeIdentity // A

func NewNodeCert( // A
	pubKey keys.PublicKey,
	issuerCAHash string,
	validFrom int64,
	validUntil int64,
	serial []byte,
	certNonce []byte,
) (*NodeCertImpl, error) {
	return certpkg.NewNodeCert(
		pubKey, issuerCAHash,
		validFrom, validUntil,
		serial, certNonce,
	)
}

func NewNodeIdentity( // A
	key *keys.AsyncCrypt,
	certs []canonical.NodeCertLike,
	caSigs [][]byte,
	authorities []EmbeddedCA,
) (*NodeIdentity, error) {
	return certpkg.NewNodeIdentity(
		key, certs, caSigs, authorities,
	)
}
