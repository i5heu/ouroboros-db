package ca

// UserCAImpl implements interfaces.UserCA with anchor
// chain validation against an AdminCA.
type UserCAImpl struct { // A
	caBase
	anchorSig       []byte
	anchorAdminHash string
}

// NewUserCA constructs a UserCAImpl. Anchor
// verification is done by the caller (AddUserPubKey).
func NewUserCA( // A
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
	cached, err := marshalPubKeyBytes(pub)
	if err != nil {
		return nil, err
	}
	return &UserCAImpl{
		caBase: caBase{
			pubKey:      pub,
			pubKeyBytes: cached,
			hash:        h,
		},
		anchorSig:       anchorSig,
		anchorAdminHash: anchorAdminHash,
	}, nil
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
