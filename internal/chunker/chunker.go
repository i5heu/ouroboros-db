package chunker

import (
	"io"

	boxochunker "github.com/ipfs/boxo/chunker"
)

// Chunker splits a stream of data into chunks.
type Chunker interface {
	// Next returns the next chunk of data.
	// It returns io.EOF when there are no more chunks.
	Next() ([]byte, error)
}

// NewChunker creates a new Chunker implementation using the default
// size splitter from boxo/chunker (256KB typically).
func NewChunker(r io.Reader) Chunker {
	return &boxoChunkerWrapper{
		splitter: boxochunker.NewSizeSplitter(r, 256*1024),
	}
}

// NewRabinChunker creates a new Chunker implementation using Rabin fingerprinting
// for content-defined chunking.
func NewRabinChunker(r io.Reader) Chunker {
	return &boxoChunkerWrapper{
		splitter: boxochunker.NewRabin(r, 256*1024),
	}
}

type boxoChunkerWrapper struct {
	splitter boxochunker.Splitter
}

func (c *boxoChunkerWrapper) Next() ([]byte, error) {
	return c.splitter.NextBytes()
}
