package interfaces

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
)

type DataRouter interface { // A
	StoreVertex(vertex Vertex) (hash.Hash, error)
	RetrieveVertex(h hash.Hash) (Vertex, error)
	DeleteVertex(h hash.Hash) error
	DistributeBlockSlices(block Block) error
	RetrieveBlock(h hash.Hash) (Block, error)
}

type BlockDistributionTracker interface { // A
	StartDistribution(block Block, walKeys [][]byte) (*BlockDistributionRecord, error)
	RecordSliceConfirmation(
		blockHash hash.Hash,
		sliceHash hash.Hash,
		nodeID string,
	) (bool, error)
	GetDistributionState(blockHash hash.Hash) (*BlockDistributionRecord, error)
	GetPendingDistributions() ([]*BlockDistributionRecord, error)
	MarkDistributed(blockHash hash.Hash) error
	MarkFailed(blockHash hash.Hash, reason string) error
	GetDistributedBlocksSince(since int64) ([]hash.Hash, error)
	GetBlockMetadata(blockHash hash.Hash) (*BlockMetadata, error)
}

type BlockStore interface { // A
	StoreBlock(block Block) error
	GetBlock(h hash.Hash) (Block, error)
	DeleteBlock(h hash.Hash) error
	StoreBlockSlice(slice BlockSlice) error
	GetBlockSlice(h hash.Hash) (BlockSlice, error)
	ListBlockSlices(blockHash hash.Hash) ([]BlockSlice, error)
	GetSealedChunkByRegion(
		blockHash hash.Hash,
		region ChunkRegion,
	) (SealedChunk, error)
	GetVertexByRegion(blockHash hash.Hash, region VertexRegion) (Vertex, error)
}

type EncryptionService interface { // A
	SealChunk(
		chunk Chunk,
		pubKeys [][]byte,
	) (SealedChunk, []KeyEntry, error)
	UnsealChunk(
		sealed SealedChunk,
		keyEntry KeyEntry,
		privKey []byte,
	) (Chunk, error)
	GenerateKeyEntry(
		chunkHash hash.Hash,
		pubKey []byte,
		aesKey []byte,
	) (KeyEntry, error)
}

type DistributedWAL interface { // A
	AppendChunk(chunk SealedChunk)
	AppendVertex(vertex Vertex)
	SealBlock() Block
}

type CAS interface { // A
	StoreContent(content []byte, parentHash hash.Hash) (Vertex, error)
	GetContent(vertexHash hash.Hash) ([]byte, error)
	DeleteContent(vertexHash hash.Hash) error
	GetVertex(h hash.Hash) (Vertex, error)
	ListChildren(parentHash hash.Hash) ([]Vertex, error)
}

type PrivilegeChecker interface { // A
	CanAccessData(authCtx AuthContext, ownerCAHash string) bool
}
