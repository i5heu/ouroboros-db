package erasure

import (
	"bytes"
	"encoding/gob"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
	rs "github.com/klauspost/reedsolomon"
)

// EncodeBlock serializes a Block and returns k+p BlockSlices.
// This is a priority function and must be reviewed before production. // PA
func EncodeBlock(block model.Block, k, p uint8) ([]model.BlockSlice, error) {
	if k == 0 {
		return nil, fmt.Errorf("erasure: k (data shards) must be > 0")
	}

	// Serialize the whole block to bytes
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(block); err != nil {
		return nil, fmt.Errorf("erasure: encode block: %w", err)
	}
	data := buf.Bytes()
	originalSize := uint64(len(data))

	// Create reedsolomon encoder
	encRS, err := rs.New(int(k), int(p))
	if err != nil {
		return nil, fmt.Errorf("erasure: new encoder: %w", err)
	}

	// Split into shards and encode parity
	shards, err := encRS.Split(data)
	if err != nil {
		return nil, fmt.Errorf("erasure: split: %w", err)
	}
	if err := encRS.Encode(shards); err != nil {
		return nil, fmt.Errorf("erasure: encode shards: %w", err)
	}

	n := int(k) + int(p)
	slices := make([]model.BlockSlice, 0, n)
	for i := 0; i < n; i++ {
		payload := make([]byte, len(shards[i]))
		copy(payload, shards[i])

		// Compose hash input: block.Hash || index || k || p || payload
		var hh bytes.Buffer
		hh.Write(block.Hash[:])
		hh.WriteByte(byte(i))
		hh.WriteByte(byte(k))
		hh.WriteByte(byte(p))
		hh.Write(payload)

		sliceHash := hash.HashBytes(hh.Bytes())

		s := model.BlockSlice{
			Hash:           sliceHash,
			BlockHash:      block.Hash,
			RSSliceIndex:   uint8(i),
			RSDataSlices:   k,
			RSParitySlices: p,
			Payload:        payload,
			OriginalSize:   originalSize,
		}
		slices = append(slices, s)
	}

	return slices, nil
}

// DecodeBlock reconstructs a Block from provided slices (need at least k).
// This is a priority function and must be reviewed before production. // PA
func DecodeBlock(slices []model.BlockSlice) (model.Block, error) {
	if len(slices) == 0 {
		return model.Block{}, fmt.Errorf("erasure: no slices")
	}

	k := int(slices[0].RSDataSlices)
	p := int(slices[0].RSParitySlices)
	n := k + p
	if n <= 0 {
		return model.Block{}, fmt.Errorf("erasure: invalid k/p")
	}

	encRS, err := rs.New(k, p)
	if err != nil {
		return model.Block{}, fmt.Errorf("erasure: new encoder: %w", err)
	}

	// Prepare shards array of length n. Nil means missing shard.
	shards := make([][]byte, n)
	var originalSize uint64
	var haveIndex bool
	for _, s := range slices {
		idx := int(s.RSSliceIndex)
		if idx < 0 || idx >= n {
			return model.Block{}, fmt.Errorf("erasure: invalid slice index %d", idx)
		}
		shards[idx] = make([]byte, len(s.Payload))
		copy(shards[idx], s.Payload)
		if !haveIndex {
			originalSize = s.OriginalSize
			haveIndex = true
		}
	}

	// Reconstruct missing shards
	if err := encRS.Reconstruct(shards); err != nil {
		return model.Block{}, fmt.Errorf("erasure: reconstruct: %w", err)
	}

	// Join shards into data
	// reedsolomon.Join requires the exact dataSize (original size).
	var out bytes.Buffer
	if err := encRS.Join(&out, shards, int(originalSize)); err != nil {
		return model.Block{}, fmt.Errorf("erasure: join: %w", err)
	}
	data := out.Bytes()

	// Decode block
	var buf bytes.Buffer
	buf.Write(data)
	dec := gob.NewDecoder(&buf)
	var block model.Block
	if err := dec.Decode(&block); err != nil {
		return model.Block{}, fmt.Errorf("erasure: decode block: %w", err)
	}

	return block, nil
}
