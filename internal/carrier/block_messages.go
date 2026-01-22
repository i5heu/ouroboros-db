// Package carrier implements inter-node communication for OuroborosDB.
package carrier

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"

	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-db/pkg/model"
)

// BlockSliceDeliveryPayload contains a block slice being delivered to a node.
type BlockSliceDeliveryPayload struct {
	// Slice is the BlockSlice being delivered.
	Slice model.BlockSlice
	// RequestID correlates the delivery with its acknowledgment.
	RequestID uint64
}

// BlockSliceAckPayload acknowledges receipt of a block slice.
type BlockSliceAckPayload struct {
	// SliceHash identifies the slice being acknowledged.
	SliceHash hash.Hash
	// BlockHash identifies the parent block.
	BlockHash hash.Hash
	// RequestID correlates with the original delivery.
	RequestID uint64
	// Success indicates whether the slice was successfully stored.
	Success bool
	// ErrorMsg contains an error message if Success is false.
	ErrorMsg string
}

// BlockAnnouncementPayload announces a newly distributed block to the cluster.
type BlockAnnouncementPayload struct {
	// BlockHash uniquely identifies the distributed block.
	BlockHash hash.Hash
	// Timestamp is when the block was created (Unix milliseconds).
	Timestamp int64
	// ChunkCount is the number of chunks in the block.
	ChunkCount uint32
	// VertexCount is the number of vertices in the block.
	VertexCount uint32
}

// MissedBlocksRequestPayload requests blocks distributed since a timestamp.
type MissedBlocksRequestPayload struct {
	// SinceTimestamp is the Unix milliseconds timestamp to query from.
	SinceTimestamp int64
}

// MissedBlocksResponsePayload returns a list of distributed block hashes.
type MissedBlocksResponsePayload struct {
	// BlockHashes lists the hashes of blocks distributed since the request time.
	BlockHashes []hash.Hash
	// Timestamps contains the creation time for each block (parallel array).
	Timestamps []int64
}

// BlockMetadataRequestPayload requests metadata for a specific block.
type BlockMetadataRequestPayload struct {
	// BlockHash identifies the block to get metadata for.
	BlockHash hash.Hash
}

// BlockMetadataResponsePayload returns block metadata.
type BlockMetadataResponsePayload struct {
	// Metadata contains the block's structural information.
	Metadata model.BlockMetadata
	// Found indicates whether the block was found.
	Found bool
}

// Serialization functions using gob encoding for simplicity and type safety.

// SerializeBlockSliceDelivery encodes a BlockSliceDeliveryPayload.
func SerializeBlockSliceDelivery(p *BlockSliceDeliveryPayload) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode block slice delivery: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlockSliceDelivery decodes a BlockSliceDeliveryPayload.
func DeserializeBlockSliceDelivery(
	data []byte,
) (*BlockSliceDeliveryPayload, error) {
	var p BlockSliceDeliveryPayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode block slice delivery: %w", err)
	}
	return &p, nil
}

// SerializeBlockSliceAck encodes a BlockSliceAckPayload.
func SerializeBlockSliceAck(p *BlockSliceAckPayload) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode block slice ack: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlockSliceAck decodes a BlockSliceAckPayload.
func DeserializeBlockSliceAck(data []byte) (*BlockSliceAckPayload, error) {
	var p BlockSliceAckPayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode block slice ack: %w", err)
	}
	return &p, nil
}

// SerializeBlockAnnouncement encodes a BlockAnnouncementPayload.
func SerializeBlockAnnouncement(p *BlockAnnouncementPayload) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode block announcement: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlockAnnouncement decodes a BlockAnnouncementPayload.
func DeserializeBlockAnnouncement(
	data []byte,
) (*BlockAnnouncementPayload, error) {
	var p BlockAnnouncementPayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode block announcement: %w", err)
	}
	return &p, nil
}

// SerializeMissedBlocksRequest encodes a MissedBlocksRequestPayload.
func SerializeMissedBlocksRequest(
	p *MissedBlocksRequestPayload,
) ([]byte, error) {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(p.SinceTimestamp))
	return buf, nil
}

// DeserializeMissedBlocksRequest decodes a MissedBlocksRequestPayload.
func DeserializeMissedBlocksRequest(
	data []byte,
) (*MissedBlocksRequestPayload, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf(
			"missed blocks request too short: %d bytes",
			len(data),
		)
	}
	return &MissedBlocksRequestPayload{
		SinceTimestamp: int64(binary.BigEndian.Uint64(data)),
	}, nil
}

// SerializeMissedBlocksResponse encodes a MissedBlocksResponsePayload.
func SerializeMissedBlocksResponse(
	p *MissedBlocksResponsePayload,
) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode missed blocks response: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeMissedBlocksResponse decodes a MissedBlocksResponsePayload.
func DeserializeMissedBlocksResponse(
	data []byte,
) (*MissedBlocksResponsePayload, error) {
	var p MissedBlocksResponsePayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode missed blocks response: %w", err)
	}
	return &p, nil
}

// hashSize is the size of a hash.Hash in bytes (SHA-512 = 64 bytes).
const hashSize = 64

// SerializeBlockMetadataRequest encodes a BlockMetadataRequestPayload.
func SerializeBlockMetadataRequest(
	p *BlockMetadataRequestPayload,
) ([]byte, error) {
	return p.BlockHash[:], nil
}

// DeserializeBlockMetadataRequest decodes a BlockMetadataRequestPayload.
func DeserializeBlockMetadataRequest(
	data []byte,
) (*BlockMetadataRequestPayload, error) {
	if len(data) < hashSize {
		return nil, fmt.Errorf(
			"block metadata request too short: %d bytes",
			len(data),
		)
	}
	var h hash.Hash
	copy(h[:], data[:hashSize])
	return &BlockMetadataRequestPayload{BlockHash: h}, nil
}

// SerializeBlockMetadataResponse encodes a BlockMetadataResponsePayload.
func SerializeBlockMetadataResponse(
	p *BlockMetadataResponsePayload,
) ([]byte, error) {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(p); err != nil {
		return nil, fmt.Errorf("encode block metadata response: %w", err)
	}
	return buf.Bytes(), nil
}

// DeserializeBlockMetadataResponse decodes a BlockMetadataResponsePayload.
func DeserializeBlockMetadataResponse(
	data []byte,
) (*BlockMetadataResponsePayload, error) {
	var p BlockMetadataResponsePayload
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&p); err != nil {
		return nil, fmt.Errorf("decode block metadata response: %w", err)
	}
	return &p, nil
}
