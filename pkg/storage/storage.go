package storage

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"fmt"
	"log"
	"sync"

	"github.com/i5heu/ouroboros-db/internal/keyValStore"
	"github.com/i5heu/ouroboros-db/pkg/types"
	"github.com/i5heu/ouroboros-db/pkg/workerPool"
)

type Storage struct {
	kv *keyValStore.KeyValStore
	wp *workerPool.WorkerPool
}

type StoreFileOptions struct {
	EventToAppendTo types.Event
	FastMeta        types.FastMeta
	Metadata        []byte
	File            []byte
	Temporary       types.Binary
	FullTextSearch  types.Binary
	WorkerPool      *workerPool.WorkerPool
}

func NewStorage(kv *keyValStore.KeyValStore, wp *workerPool.WorkerPool) StorageService {
	return &Storage{
		kv: kv,
		wp: wp,
	}
}

func (s *Storage) Close() {
	s.kv.Close()
}

// will store the file in the chunkStore and create new Event as child of given event
func (s *Storage) StoreFile(options StoreFileOptions) (types.Event, error) {
	// Validate options before proceeding
	err := options.ValidateOptions()
	if err != nil {
		log.Fatalf("Error validating options: %v", err)
		return types.Event{}, err
	}

	// Create channels to handle asynchronous results and errors
	fileChunkKeysChan := make(chan types.ChunkMetaCollection, 1)
	metadataChunkKeysChan := make(chan types.ChunkMetaCollection, 1)
	errorChan := make(chan error, 2) // buffer for two possible errors

	var wg sync.WaitGroup
	wg.Add(2)

	// Store file data in chunk store asynchronously
	go func() {
		defer wg.Done()
		keys, err := s.storeDataInChunkStore(options.File)
		if err != nil {
			errorChan <- err
			return
		}
		fileChunkKeysChan <- keys
	}()

	// Store metadata in chunk store asynchronously
	go func() {
		defer wg.Done()
		keys, err := s.storeDataInChunkStore(options.Metadata)
		if err != nil {
			errorChan <- err
			return
		}
		metadataChunkKeysChan <- keys
	}()

	// Wait for both goroutines to complete
	wg.Wait()
	close(errorChan)
	close(fileChunkKeysChan)
	close(metadataChunkKeysChan)

	// Check for errors
	for err := range errorChan {
		log.Printf("Error in storing data: %v", err)
		return types.Event{}, err
	}

	// Retrieve results from channels
	fileChunkKeys := <-fileChunkKeysChan
	metadataChunkKeys := <-metadataChunkKeysChan

	// Create a new event
	newEvent, err := s.CreateNewEvent(EventOptions{
		EventType:      types.Normal,
		ParentEvent:    options.EventToAppendTo.EventIdentifier.EventHash,
		RootEvent:      options.EventToAppendTo.RootEvent,
		FastMeta:       types.FastMeta{},
		Metadata:       metadataChunkKeys,
		Content:        fileChunkKeys,
		Temporary:      options.Temporary,
		FullTextSearch: options.FullTextSearch,
	})
	if err != nil {
		log.Fatalf("Error creating new event: %v", err)
		return types.Event{}, err
	}

	return newEvent, nil
}

func (options *StoreFileOptions) ValidateOptions() error {
	if options.EventToAppendTo.EventIdentifier.EventHash == (types.Hash{}) {
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

func (s *Storage) GetFile(eventOfFile types.Event) ([]byte, error) {
	data, err := s.getData(eventOfFile.Content)
	if err != nil {
		return nil, fmt.Errorf("Error getting File: %v", err)
	}

	return data, nil
}

func (s *Storage) GetMetadata(eventOfFile types.Event) ([]byte, error) {

	data, err := s.getData(eventOfFile.Metadata)
	if err != nil {
		return nil, fmt.Errorf("Error getting Metadata: %v", err)
	}

	return data, nil
}

func (s *Storage) getData(chunkCollection types.ChunkMetaCollection) ([]byte, error) {
	data := []byte{}

	pass := "HelloWorld"
	key := sha256.Sum256([]byte(pass))
	c, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, fmt.Errorf("Error creating cipher: %v", err)
	}

	gcm, err := cipher.NewGCM(c)
	if err != nil {
		return nil, err
	}

	for _, chunk := range chunkCollection {
		chunkData, err := s.kv.Read(chunk.Hash.Bytes())
		if err != nil {
			return nil, fmt.Errorf("Error reading chunk from GetFile: %v", err)
		}

		iv := chunk.Hash.Bytes()[:gcm.NonceSize()]

		encryptedCompressed, err := gcm.Open(nil, iv, chunkData, nil)
		if err != nil {
			return nil, fmt.Errorf("Error decrypting chunk: %v", err)
		}

		decompressed, err := decompressWithLzma(encryptedCompressed)
		if err != nil {
			return nil, fmt.Errorf("Error decompressing chunk: %v", err)
		}

		data = append(data, decompressed...)
	}

	return data, nil
}

func (s *Storage) storeDataInChunkStore(data []byte) (types.ChunkMetaCollection, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("Error storing data: Data is empty")
	}

	chunks, chunkMetaCollection, err := s.StoreDataPipeline(data)
	if err != nil {
		log.Fatalf("Error chunking data: %v", err)
		return nil, err
	}

	err = s.kv.BatchWriteNonExistingChunks(chunks)
	if err != nil {
		log.Fatalf("Error writing chunks: %v", err)
		return nil, err
	}

	return chunkMetaCollection, nil
}

func (s *Storage) GarbageCollection() error {
	return s.kv.Clean()
}
