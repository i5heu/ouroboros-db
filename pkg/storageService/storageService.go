package storageService

import (
	"OuroborosDB/pkg/buzhashChunker"
	"OuroborosDB/pkg/keyValStore"
	"fmt"
	"log"
)

type StoreFileOptions struct {
	EventToAppendTo Event
	Metadata        []byte
	File            []byte
	Temporary       bool
	FullTextSearch  bool
}

// will store the file in the chunkStore and create new Event as child of given event
func StoreFile(kv keyValStore.KeyValStore, options StoreFileOptions) (Event, error) {
	err := options.ValidateOptions()
	if err != nil {
		log.Fatalf("Error validating options: %v", err)
		return Event{}, err
	}

	fileChunkKeys, err := storeDataInChunkStore(kv, options.File)
	if err != nil {
		log.Fatalf("Error storing file: %v", err)
		return Event{}, err
	}

	metadataChunkKeys, err := storeDataInChunkStore(kv, options.Metadata)
	if err != nil {
		log.Fatalf("Error storing metadata: %v", err)
		return Event{}, err
	}

	newEvent, err := CreateNewEvent(kv, EventOptions{
		ContentHashes:     fileChunkKeys,
		MetadataHashes:    metadataChunkKeys,
		HashOfParentEvent: options.EventToAppendTo.EventHash,
		Temporary:         options.Temporary,
		FullTextSearch:    options.FullTextSearch,
	})

	if err != nil {
		log.Fatalf("Error creating new event: %v", err)
		return Event{}, err
	}

	return newEvent, nil
}

func (options *StoreFileOptions) ValidateOptions() error {
	if options.EventToAppendTo.EventHash == [64]byte{} {
		return fmt.Errorf("Error storing file: Parent event was not defined")
	}

	if len(options.File) == 0 && len(options.Metadata) == 0 {
		return fmt.Errorf("Error storing file: Both file and metadata are empty")
	}

	if !options.Temporary && options.EventToAppendTo.Temporary {
		return fmt.Errorf("Error storing file: Parent event is Temporary and can not have non-Temporary children")
	}

	return nil
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
