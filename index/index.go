package index

import (
	"sync/atomic"

	"github.com/blevesearch/bleve/v2"
	ouroboroskv "github.com/i5heu/ouroboros-kv"
)

//TODO : implement thread root to last child edit timestamp to optimize sorting by last activity

//TODO: implement semantic text search with bleve vector search and Qwen/Qwen3-Embedding-0.6B embeddings
//TODO: check if it would be better to have a bleve on disk instead of in memory only

type Indexer struct {
	kv atomic.Pointer[ouroboroskv.KV]
	bi bleve.Index
}

func NewIndexer() *Indexer {
	index, err := bleve.NewMemOnly(bleve.NewIndexMapping())
	if err != nil {
		panic(err)
	}

	_ = index
	return &Indexer{
		bi: index,
	}
}
