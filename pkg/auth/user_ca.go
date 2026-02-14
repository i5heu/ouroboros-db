package auth

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// UserCAImpl implements interfaces.UserCA with anchor
// chain validation against an AdminCA.
type UserCAImpl struct { // A
	pubKey          *keys.PublicKey
	hash            string
	anchorSig       []byte
	anchorAdminHash string
}

// newUserCA constructs a UserCAImpl. Anchor
// verification is done by the caller (AddUserPubKey).
func newUserCA( // A
	pubKeyBytes []byte,
	anchorSig []byte,
	anchorAdminHash string,
) (*UserCAImpl, error) {
	pub, err := splitAndParsePubKey(pubKeyBytes)
	if err != nil {
		return nil, err
	}
	h, err := caHash(pub)
	if err != nil {
		return nil, err
	}
	return &UserCAImpl{
		pubKey:          pub,
		hash:            h,
		anchorSig:       anchorSig,
		anchorAdminHash: anchorAdminHash,
	}, nil
}

// PubKey returns the concatenated KEM+Sign bytes.
func (u *UserCAImpl) PubKey() []byte { // A
	kem, err := u.pubKey.MarshalBinaryKEM()
	if err != nil {
		return nil
	}
	sign, err := u.pubKey.MarshalBinarySign()
	if err != nil {
		return nil
	}
	out := make([]byte, len(kem)+len(sign))
	copy(out, kem)
	copy(out[len(kem):], sign)
	return out
}

// Hash returns hex(SHA-256(signPubKeyBytes)).
func (u *UserCAImpl) Hash() string { // A
	return u.hash
}

// AnchorSig returns the anchor signature bytes.
func (u *UserCAImpl) AnchorSig() []byte { // A
	return u.anchorSig
}

// AnchorAdminHash returns the hash of the AdminCA
// that anchored this UserCA.
func (u *UserCAImpl) AnchorAdminHash() string { // A
	return u.anchorAdminHash
}

// VerifyNodeCert verifies a CA signature on a
// NodeCert and returns the derived NodeID.
func (u *UserCAImpl) VerifyNodeCert( // A
	cert NodeCertLike,
	caSignature []byte,
) (keys.NodeID, error) {
	a := &AdminCAImpl{pubKey: u.pubKey, hash: u.hash}
	return a.VerifyNodeCert(cert, caSignature)
}
