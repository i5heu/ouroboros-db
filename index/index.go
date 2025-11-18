package index

import (
	"sync/atomic"

	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

//TODO : implement thread root to last child edit timestamp to optimize sorting by last activity

//TODO: implement semantic text search with bleve vector search and Qwen/Qwen3-Embedding-0.6B embeddings

type Indexer struct {
	kv atomic.Pointer[ouroboroskv.KV]
	
}
