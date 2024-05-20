package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/i5heu/ouroboros-db/pkg/buzhashChunker"
)

func main() {

	// check if the user has provided the file path
	if len(os.Args) < 2 {
		fmt.Println("Please provide the file path")
		os.Exit(1)
	}

	// get the input from the user
	filePath := os.Args[1]

	// read the file
	file, err := os.Open(filePath)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	// create a new chunker
	chunks, metas, err := buzhashChunker.ChunkReaderSynchronously(file)
	if err != nil {
		panic(err)
	}

	// create folder to store the chunks besides the original file
	err = os.Mkdir(filePath+"_chunks", 0755)
	if err != nil {
		panic(err)
	}

	// write the chunks to the folder
	for i, chunk := range chunks {
		chunkFile, err := os.Create(filePath + "_chunks/chunk_" + fmt.Sprintf("%d", i))
		if err != nil {
			panic(err)
		}
		defer chunkFile.Close()
		chunkFile.Write(chunk.Data)
	}

	// write the meta into a file in the folder
	metaFile, err := os.Create(filePath + "_chunks/meta")
	if err != nil {
		panic(err)
	}
	defer metaFile.Close()
	// store as json
	metaJSON, err := json.Marshal(metas)
	if err != nil {
		panic(err)
	}
	metaFile.Write(metaJSON)

	fmt.Println("Chunks and meta data are stored in the folder: ", filePath+"_chunks")

}
