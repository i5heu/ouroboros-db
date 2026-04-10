package cas

import "github.com/i5heu/ouroboros-crypt/pkg/hash"

type Scope struct {
	ScopeHash           hash.Hash
	CertHash            hash.Hash // Hash of the certificate that can access and decrypt this scope
	EncryptedCiphertext [32]byte  // 1568 bytes for ML-KEM-1024 encrypted Ciphertext(32-byte)
}

type ScopeLink struct {
	ScopeHash hash.Hash // -> lead to ML-KEM-1024 encrypted Ciphertext(32-byte): 1568 bytes
	AESNonce  [12]byte  // 12 bytes
}

// each hash.Hash is 64 bytes

// 4*64 + 12 = 268 bytes for each Data

// Id we reduce hash.Hash to 32 bytes, then each Data is 32*4 + 12 = 140 bytes // USE SHA-512/256

type Data struct {
	ContentHash hash.Hash
	ParentHash  hash.Hash

	ReliabilityDesire     uint8
	AccessFrequencyDesire uint8
	Scopes                []ScopeLink
	Indexed               bool // Then just append the index vectors or data to the child block
	IndexData             bool // If true, then the content is the index data.

	EncryptedContent []byte

	DataHash hash.Hash // Hash of all the above fields
}
