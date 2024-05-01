package storageService

import (
	"OuroborosDB/pkg/buzhashChunker"
	"OuroborosDB/pkg/keyValStore"
	"log"
)

// will store the file in the chunkStore and create new Event as child of given event
func StoreFile(kv keyValStore.KeyValStore, eventToAppendTo Event, metadata []byte, file []byte) (Event, error) {
	fileChunkKeys, err := storeDataInChunkStore(kv, file)
	if err != nil {
		log.Fatalf("Error storing file: %v", err)
		return Event{}, err
	}

	metadataChunkKeys, err := storeDataInChunkStore(kv, metadata)
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return Event{}, err
	}

	newEvent, err := CreateNewEvent(kv, eventToAppendTo.EventHash, metadataChunkKeys, fileChunkKeys)
	if err != nil {
		log.Fatalf("Error creating new event: %v", err)
		return Event{}, err
	}

	return newEvent, nil
}

func GetFile(kv keyValStore.KeyValStore, eventOfFile Event) ([]byte, error) {
	file := []byte{}

	for _, key := range eventOfFile.ContentHashes {
		chunk, err := kv.Read(key[:])
		if err != nil {
			log.Fatalf("Error reading chunk: %v", err)
			return nil, err
		}

		file = append(file, chunk...)
	}

	return file, nil
}

func GetMetadata(kv keyValStore.KeyValStore, eventOfFile Event) ([]byte, error) {
	metadata := []byte{}

	for _, key := range eventOfFile.MetadataHashes {
		chunk, err := kv.Read(key[:])
		if err != nil {
			log.Fatalf("Error reading chunk: %v", err)
			return nil, err
		}

		metadata = append(metadata, chunk...)
	}

	return metadata, nil
}

func storeDataInChunkStore(kv keyValStore.KeyValStore, data []byte) ([][64]byte, error) {
	chunks, err := buzhashChunker.ChunkBytes(data)
	if err != nil {
		log.Fatalf("Error chunking data: %v", err)
		return nil, err
	}

	keys := make([][64]byte, 0)

	for _, chunk := range chunks {
		keys = append(keys, chunk.Hash)
		err := kv.Write(chunk.Hash[:], chunk.Chunk)
		if err != nil {
			log.Fatalf("Error writing chunk: %v", err)
			return nil, err
		}
	}

	return keys, nil
}
