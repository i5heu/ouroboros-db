package main

import (
	"crypto/rand"
)

func main() {

}

func generateRandomHash() [64]byte {
	var hash [64]byte
	_, err := rand.Read(hash[:])
	if err != nil {
		panic(err) // Handle errors better in production code
	}
	return hash
}

func generateRandomHashes(n int) [][64]byte {
	hashes := make([][64]byte, n)
	for i := range hashes {
		hashes[i] = generateRandomHash()
	}
	return hashes
}
