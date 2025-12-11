package cas

import (
	"context"
	"errors"
	"fmt"

	crypt "github.com/i5heu/ouroboros-crypt"
	"github.com/i5heu/ouroboros-crypt/pkg/encrypt"
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/klauspost/compress/zstd"
	"github.com/klauspost/reedsolomon"
)

const maxSliceCount = 256 // the maximum of uint8 because RSSliceIndex is uint8

type keyIndex interface {
	Get(
		sealedSliceHash, pubKeyHash hash.Hash,
	) (MlKemEncapsulatedSecret [][]byte, err error)
	Set(
		sealedSliceHash, pubKeyHash hash.Hash,
		MlKemEncapsulatedSecret []byte,
	) error
}

// SealedSlice represents a single Reed-Solomon slice (data or parity) persisted
// in the key-value
// store.
// It contains the compressed,sliced, and encrypted data of a chunk.
type SealedSlice struct {
	cas *CAS

	// Hash constructed via the generateHash method
	// The Hash must be generated with this information in this order:
	// ChunkHash,RSDataSlices,RSParitySlices,RSSliceIndex,Nonce,SealedPayload
	Hash hash.Hash

	ChunkHash      hash.Hash // Hash of the originating chunk
	RSDataSlices   uint8     // Number of Data slices (k)
	RSParitySlices uint8     // Number of Parity slices (p = n - k)

	// Index of the slice within the stripe
	// (data slices must precede parity slices)
	// example: for a stripe with 4 data and 2 parity slices,
	// data(k) slice indices are 0,1,2,3 and parity(p) slice indices are 4,5
	RSSliceIndex uint8

	Nonce         []byte // AES-GCM nonce for encryption
	sealedPayload []byte // cache for encrypted slice payload
}

func NewSealedSlice(
	cas *CAS,
	chunkHash hash.Hash,
	rsDataSlices, rsParitySlices, rsSliceIndex uint8,
	nonce []byte,
) SealedSlice {
	return SealedSlice{
		cas:            cas,
		ChunkHash:      chunkHash,
		RSDataSlices:   rsDataSlices,
		RSParitySlices: rsParitySlices,
		RSSliceIndex:   rsSliceIndex,
		Nonce:          nonce,
	}
}

func (s *SealedSlice) GetSealedPayload(ctx context.Context) ([]byte, error) {
	if s.sealedPayload != nil {
		return s.sealedPayload, nil
	}

	payload, err := s.cas.dr.GetSealedSlicePayload(ctx, s.Hash)
	if err != nil {
		return nil, fmt.Errorf("cas: failed to get sealed slice payload: %w", err)
	}
	s.sealedPayload = payload
	return s.sealedPayload, nil
}

func (s *SealedSlice) Decrypt(
	ctx context.Context,
	c crypt.Crypt,
) ([]byte, error) {
	pubKeyHash, err := c.Encryptor.PublicKey.Hash()
	if err != nil {
		return nil, err
	}

	encapsulatedSecret, err := s.cas.ki.Get(s.Hash, pubKeyHash)
	if err != nil {
		return nil, err
	}

	_, err = s.GetSealedPayload(ctx)
	if err != nil {
		return nil, err
	}

	clearBytes, err := c.Decrypt(&encrypt.EncryptResult{
		Ciphertext:      s.sealedPayload,
		Nonce:           s.Nonce,
		EncapsulatedKey: encapsulatedSecret[0], // TODO: support multiple keys
	})
	if err != nil {
		return nil, err
	}

	return clearBytes, nil
}

func (s *SealedSlice) GetHash(ctx context.Context) (hash.Hash, error) {
	return s.generateHash(ctx, false)
}

func (s *SealedSlice) ValidateHash(ctx context.Context) (hash.Hash, error) {
	return s.generateHash(ctx, true)
}

func (s *SealedSlice) generateHash( // H
	ctx context.Context,
	validate bool,
) (hash.Hash, error) {
	// If the hash is already set, return it.
	if s.Hash != (hash.Hash{}) && !validate {
		return s.Hash, nil
	}

	// Validate that all required fields are present
	err := s.validateDataForHashGeneration(ctx)
	if err != nil {
		return hash.Hash{}, err
	}

	_, err = s.GetSealedPayload(ctx)
	if err != nil {
		return hash.Hash{}, err
	}

	// Generate the hash
	buf := make([]byte, 0, 3+len(s.Nonce)+len(s.sealedPayload)+len(s.ChunkHash))
	buf = append(buf, s.ChunkHash[:]...)
	buf = append(buf, s.RSDataSlices, s.RSParitySlices, s.RSSliceIndex)
	buf = append(buf, s.Nonce...)
	buf = append(buf, s.sealedPayload...)

	newHash := hash.HashBytes(buf)

	// If validating, compare the newly generated hash with the existing one
	if validate && s.Hash != (hash.Hash{}) && newHash != s.Hash {
		return hash.Hash{}, fmt.Errorf(
			"cas: hash validation failed: stored=%s computed=%s",
			s.Hash.String(),
			newHash.String(),
		)
	}

	s.Hash = newHash
	return s.Hash, nil
}

func (s *SealedSlice) validateDataForHashGeneration(
	ctx context.Context,
) error { // H
	// Check that required fields are present
	if s.ChunkHash == (hash.Hash{}) {
		return errors.New(
			"cas: missing ChunkHash field for hash generation",
		)
	}

	if len(s.Nonce) == 0 {
		return errors.New("cas: missing Nonce field for hash generation")
	}

	_, err := s.GetSealedPayload(ctx)
	if err != nil {
		return fmt.Errorf(
			"cas: failed to get sealed payload for hash generation: %w",
			err,
		)
	}

	if len(s.sealedPayload) == 0 {
		return errors.New("cas: missing Payload field for hash generation")
	}

	if s.RSDataSlices == 0 {
		return errors.New(
			"cas: missing RSDataSlices field for hash generation",
		)
	}

	if s.RSParitySlices == 0 {
		return errors.New(
			"cas: missing RSParitySlices field for hash generation",
		)
	}
	return nil
}

type storeSealedSlicesFromChunkOpts struct {
	CAS             *CAS
	Crypt           crypt.Crypt
	ClearChunkBytes []byte
	ChunkHash       hash.Hash
	RSDataSlices    uint8
	RSParitySlices  uint8
}

func (s *storeSealedSlicesFromChunkOpts) Validate() error { // H
	if s.CAS == nil {
		return errors.New("cas: CAS must be provided")
	}
	if s.Crypt.Encryptor == nil {
		return errors.New("cas: encryptor must be provided")
	}
	if len(s.ClearChunkBytes) == 0 {
		return errors.New("cas: ClearChunkBytes must be provided")
	}
	if s.ChunkHash == (hash.Hash{}) {
		return errors.New("cas: ChunkHash must be provided")
	}
	if s.RSDataSlices == 0 {
		return errors.New("cas: RSDataSlices must be greater than zero")
	}
	if s.RSParitySlices == 0 {
		return errors.New("cas: RSParitySlices must be greater than zero")
	}
	return nil
}

func storeSealedSlicesFromChunk( // H
	ctx context.Context,
	opts storeSealedSlicesFromChunkOpts,
) ([]SealedSlice, error) {
	compressedChunk, err := compressChunk(opts.ClearChunkBytes)
	if err != nil {
		return nil, err
	}

	shards, err := createReedSolomonShards(
		compressedChunk,
		opts.RSDataSlices,
		opts.RSParitySlices,
	)
	if err != nil {
		return nil, err
	}

	err = validateShardCount(shards, opts.RSDataSlices, opts.RSParitySlices)
	if err != nil {
		return nil, err
	}

	var sealedSlices []SealedSlice
	for i, shard := range shards {
		slice, err := encryptAndSealSlice(
			ctx,
			opts,
			shard,
			opts.ChunkHash,
			//nolint:gosec // G115: validateShardCount, so sliceIndex <= 255
			uint8(i),
		)
		if err != nil {
			return nil, err
		}
		sealedSlices = append(sealedSlices, *slice)
	}

	for _, s := range sealedSlices {
		err = opts.CAS.dr.SetSealedSlice(ctx, s)
		if err != nil {
			return nil, err
		}
	}

	return sealedSlices, nil
}

func validateShardCount( // H
	shards [][]byte,
	rsDataSlices, rsParitySlices uint8,
) error {
	if len(shards) > int(rsDataSlices)+int(rsParitySlices) {
		return fmt.Errorf(
			"cas: number of generated slices exceeds maximum allowed: got %d, max %d",
			len(shards),
			int(rsDataSlices)+int(rsParitySlices),
		)
	}
	if len(shards) > maxSliceCount {
		return fmt.Errorf(
			"cas: number of generated slices exceeds uint8 maximum: got %d, max %d",
			len(shards),
			maxSliceCount,
		)
	}
	return nil
}

func compressChunk(clearChunk []byte) ([]byte, error) { // AC
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return nil, err
	}
	compressedChunk := encoder.EncodeAll(clearChunk, nil)
	err = encoder.Close()
	if err != nil {
		return nil, errors.New("cas: failed to close zstd encoder: " + err.Error())
	}
	return compressedChunk, nil
}

func createReedSolomonShards( // AC
	compressedData []byte,
	rsDataSlices, rsParitySlices uint8,
) ([][]byte, error) {
	enc, err := reedsolomon.New(int(rsDataSlices), int(rsParitySlices))
	if err != nil {
		return nil, err
	}

	shards, err := enc.Split(compressedData)
	if err != nil {
		return nil, err
	}

	err = enc.Encode(shards)
	if err != nil {
		return nil, err
	}

	return shards, nil
}

func encryptAndSealSlice( // AC
	ctx context.Context,
	opts storeSealedSlicesFromChunkOpts,
	shard []byte,
	chunkHash hash.Hash,
	sliceIndex uint8,
) (*SealedSlice, error) {
	encryptResult, err := opts.Crypt.Encrypt(shard)
	if err != nil {
		return &SealedSlice{}, err
	}

	slice := &SealedSlice{
		cas:            opts.CAS,
		ChunkHash:      chunkHash,
		RSDataSlices:   opts.RSDataSlices,
		RSParitySlices: opts.RSParitySlices,
		RSSliceIndex:   uint8(sliceIndex),
		Nonce:          encryptResult.Nonce,
		sealedPayload:  encryptResult.Ciphertext,
	}

	pubKeyHash, err := opts.Crypt.Encryptor.PublicKey.Hash()
	if err != nil {
		return &SealedSlice{}, err
	}

	_, err = slice.generateHash(ctx, true)
	if err != nil {
		return &SealedSlice{}, err
	}

	err = opts.CAS.ki.Set(
		slice.Hash,
		pubKeyHash,
		encryptResult.EncapsulatedKey,
	)
	if err != nil {
		return &SealedSlice{}, err
	}

	return slice, nil
}
