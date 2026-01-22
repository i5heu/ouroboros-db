package erasure

import (
	"bytes"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/carrier"
	"github.com/i5heu/ouroboros-db/pkg/model"
	"github.com/klauspost/reedsolomon"
)

// ErasureEncoder handles Reed-Solomon encoding and decoding.
type ErasureEncoder struct {
	enc reedsolomon.Encoder
}

// NewErasureEncoder creates a new encoder with the specified data and parity shards.
func NewErasureEncoder(dataShards, parityShards int) (*ErasureEncoder, error) {
	enc, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("create reedsolomon encoder: %w", err)
	}
	return &ErasureEncoder{enc: enc}, nil
}

// Encode splits data into data+parity shards.
func (e *ErasureEncoder) Encode(data []byte) ([][]byte, error) {
	shards, err := e.enc.Split(data)
	if err != nil {
		return nil, fmt.Errorf("split data: %w", err)
	}
	if err := e.enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("encode shards: %w", err)
	}
	return shards, nil
}

// Reconstruct reconstructs the data from the shards.
// The shards slice must contain data+parity shards. Missing shards can be nil.
// It modifies the shards slice in place (filling nil shards).
func (e *ErasureEncoder) Reconstruct(shards [][]byte) error {
	return e.enc.Reconstruct(shards)
}

// Join joins the shards back into the original data.
// It assumes the shards are already reconstructed if necessary.
// 'outSize' is the size of the original data (needed to trim padding).
func (e *ErasureEncoder) Join(shards [][]byte, outSize int) ([]byte, error) {
	var buf bytes.Buffer
	if err := e.enc.Join(&buf, shards, outSize); err != nil {
		return nil, fmt.Errorf("join shards: %w", err)
	}
	return buf.Bytes(), nil
}

// EncodeBlock serializes a block and splits it into Reed-Solomon slices.
func EncodeBlock(block model.Block, dataSlices, paritySlices uint8) ([]model.BlockSlice, error) {
	// 1. Serialize the block
	data, err := carrier.Serialize(block)
	if err != nil {
		return nil, fmt.Errorf("serialize block: %w", err)
	}

	// 2. Create encoder
	enc, err := NewErasureEncoder(int(dataSlices), int(paritySlices))
	if err != nil {
		return nil, err
	}

	// 3. Encode into shards
	shards, err := enc.Encode(data)
	if err != nil {
		return nil, err
	}

	// 4. Create BlockSlice objects
	slices := make([]model.BlockSlice, 0, len(shards))
	for i, shard := range shards {
		sliceHash := hash.HashBytes(shard)
		slices = append(slices, model.BlockSlice{
			Hash:           sliceHash,
			BlockHash:      block.Hash,
			RSSliceIndex:   uint8(i),
			RSDataSlices:   dataSlices,
			RSParitySlices: paritySlices,
			Payload:        shard,
		})
	}

	return slices, nil
}
