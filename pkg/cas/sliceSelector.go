package cas

import (
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

// reconstructionParams holds the validated parameters for reconstruction.
type reconstructionParams struct { // A
	chunkHash hash.Hash
	k         int
	p         int
	total     int
}

// validateReconstructionParameters validates the first slice and RS parameters.
func validateReconstructionParameters( // A
	first SealedSlice,
) (*reconstructionParams, error) {
	k := int(first.RSDataSlices)
	p := int(first.RSParitySlices)

	if k <= 0 {
		return nil, fmt.Errorf(
			"invalid RSDataSlices=%d",
			k,
		)
	}
	if p < 0 {
		return nil, fmt.Errorf(
			"invalid RSParitySlices=%d",
			p,
		)
	}

	total := k + p
	if total == 0 {
		return nil, fmt.Errorf(
			"total slices (k+p) is zero",
		)
	}

	return &reconstructionParams{
		chunkHash: first.ChunkHash,
		k:         k,
		p:         p,
		total:     total,
	}, nil
}

// validateAndAddSlice validates a single slice against parameters and adds it
// to selected.
// Returns true if a new distinct slice was added.
func validateAndAddSlice( // A
	s SealedSlice,
	params *reconstructionParams,
	first SealedSlice,
	selected []*SealedSlice,
) (bool, error) {
	// 1) Enforce same chunk
	if s.ChunkHash != params.chunkHash {
		return false, fmt.Errorf(
			"mixed ChunkHash in input",
		)
	}

	// 2) Enforce consistent RS parameters
	if s.RSDataSlices != first.RSDataSlices ||
		s.RSParitySlices != first.RSParitySlices {
		return false, fmt.Errorf(
			"mixed RS parameters in input (got %d/%d, expected %d/%d)",
			s.RSDataSlices,
			s.RSParitySlices,
			first.RSDataSlices,
			first.RSParitySlices,
		)
	}

	idx := int(s.RSSliceIndex)
	if idx < 0 || idx >= params.total {
		return false, fmt.Errorf(
			"RSSliceIndex %d out of range [0,%d)",
			idx,
			params.total,
		)
	}

	if len(s.Nonce) == 0 {
		return false, fmt.Errorf(
			"empty nonce for slice index %d",
			idx,
		)
	}

	// Add if not already present
	if selected[idx] == nil {
		sliceCopy := s
		selected[idx] = &sliceCopy
		return true, nil
	}

	return false, nil
}

// buildReconstructionResult builds the final result from selected slices.
func buildReconstructionResult( // A
	selected []*SealedSlice,
	total int,
) []SealedSlice {
	distinctCount := 0
	for _, s := range selected {
		if s != nil {
			distinctCount++
		}
	}

	result := make([]SealedSlice, 0, distinctCount)
	for i := 0; i < total; i++ {
		if selected[i] != nil {
			result = append(result, *selected[i])
		}
	}
	return result
}

// selectSealedSlicesForReconstruction filters and normalizes the set of slices
// so that reconstruction can be done safely and efficiently.
//
// Invariants enforced:
//   - All returned slices belong to the same chunk (ChunkHash)
//
// - All returned slices use the same RS parameters (RSDataSlices,
// RSParitySlices)
//   - At most one slice per RSSliceIndex is returned
//   - We return an error if there are fewer distinct slices than RSDataSlices
//
// Complexity: O(len(sealedSlices)) time, O(k+p) additional memory.
func selectSealedSlicesForReconstruction( // A
	sealedSlices []SealedSlice,
) ([]SealedSlice, error) {
	if len(sealedSlices) == 0 {
		return nil, fmt.Errorf(
			"no slices provided",
		)
	}

	first := sealedSlices[0]
	params, err := validateReconstructionParameters(first)
	if err != nil {
		return nil, err
	}

	// selected[idx] holds the chosen slice for RSSliceIndex == idx
	selected := make([]*SealedSlice, params.total)
	distinctCount := 0

	for _, s := range sealedSlices {
		added, err := validateAndAddSlice(s, params, first, selected)
		if err != nil {
			return nil, err
		}
		if added {
			distinctCount++
		}
	}

	// Reed–Solomon reconstruction requires at least k available slices.
	if distinctCount < params.k {
		return nil, fmt.Errorf(
			"not enough slices for reconstruction: have %d distinct, need %d",
			distinctCount,
			params.k,
		)
	}

	return buildReconstructionResult(selected, params.total), nil
}
