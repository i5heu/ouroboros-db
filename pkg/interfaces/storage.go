package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type Vertex struct { // A
	Hash        hash.Hash
	Parent      hash.Hash
	Created     int64
	ChunkHashes []hash.Hash
}

func (v *Vertex) GetContent() []byte { // A
	return nil
}

type Chunk struct { // A
	Hash    hash.Hash
	Size    int
	Content []byte
}

func (c *Chunk) GetContent() []byte { // A
	return c.Content
}

type SealedChunk struct { // A
	ChunkHash        hash.Hash
	SealedHash       hash.Hash
	EncryptedContent []byte
	Nonce            []byte
	OriginalSize     int
}

type KeyEntry struct { // A
	ChunkHash          hash.Hash
	PubKeyHash         hash.Hash
	EncapsulatedAESKey []byte
}

type ChunkRegion struct { // A
	ChunkHash hash.Hash
	Offset    uint32
	Length    uint32
}

type VertexRegion struct { // A
	VertexHash hash.Hash
	Offset     uint32
	Length     uint32
}

type BlockHeader struct { // A
	Version        uint8
	Created        int64
	RSDataSlices   uint8
	RSParitySlices uint8
	ChunkCount     uint32
	VertexCount    uint32
	TotalSize      uint32
}

type Block struct { // A
	Hash          hash.Hash
	Header        BlockHeader
	DataSection   []byte
	VertexSection []byte
	KeyRegistry   map[hash.Hash][]KeyEntry
}

type BlockSlice struct { // A
	Hash           hash.Hash
	BlockHash      hash.Hash
	RSSliceIndex   uint8
	RSDataSlices   uint8
	RSParitySlices uint8
	Payload        []byte
}
