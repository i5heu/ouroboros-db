package auth

import (
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
	capkg "github.com/i5heu/ouroboros-db/internal/auth/ca"
)

const KEMPublicKeySize = capkg.KEMPublicKeySize // A

const CTXNodeAdmissionV1 = capkg.CTXNodeAdmissionV1 // A

const CTXUserCAAnchorV1 = capkg.CTXUserCAAnchorV1 // A

var ErrInvalidCASignature = capkg.ErrInvalidCASignature // A

type AdminCAImpl = capkg.AdminCAImpl // A

type UserCAImpl = capkg.UserCAImpl // A

func NewAdminCA( // A
	pubKeyBytes []byte,
) (*AdminCAImpl, error) {
	return capkg.NewAdminCA(pubKeyBytes)
}

func newUserCA( // A
	pubKeyBytes []byte,
	anchorSig []byte,
	anchorAdminHash string,
) (*UserCAImpl, error) {
	return capkg.NewUserCA(
		pubKeyBytes, anchorSig, anchorAdminHash,
	)
}

func MarshalPubKeyBytes( // A
	pub *keys.PublicKey,
) ([]byte, error) {
	return capkg.MarshalPubKeyBytes(pub)
}
