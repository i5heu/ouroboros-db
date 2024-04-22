package ouroborosMQ

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"OuroborosDB/internal/config"
)

type EventData struct {
	Key  string
	Type string
	Data []byte
}

type Event struct {
	EncryptedDataHash string
	Timestamp         time.Time
	ParentEventHash   string
	Temporary         bool
	SourcePriority    uint64
	SourceEvents      []string // hashes of source events
	EncryptedData     []byte
}

func CreateEvent(eventData EventData, key string) Event {
	// Create a buffer to hold the encoded data
	var buf bytes.Buffer

	// Create an encoder that writes to the buffer
	encoder := gob.NewEncoder(&buf)

	// Encode the originalData into the buffer
	err := encoder.Encode(eventData)
	if err != nil {
		fmt.Println("Error encoding EventData:", err)
		return Event{}
	}

	encryptedDataSerilazed, err := encrypt(buf.Bytes(), []byte(key))
	if err != nil {
		fmt.Println("Error encrypting data:", err)
		return Event{}
	}

	// get SHA256 hash of the data

	return Event{
		EncryptedDataHash: "hash",
		Timestamp:         time.Now(),
		ParentEventHash:   "0",
		Temporary:         false,
		SourcePriority:    0,
		SourceEvents:      []string{"hash"},
		EncryptedData:     encryptedDataSerilazed,
	}
}

func SerializeEvent(event Event) []byte {
	// Create a buffer to hold the encoded data
	var buf bytes.Buffer

	// Create an encoder that writes to the buffer
	encoder := gob.NewEncoder(&buf)

	// Encode the originalData into the buffer
	err := encoder.Encode(event)
	if err != nil {
		fmt.Println("Error encoding Event:", err)
		return []byte{}
	}

	return buf.Bytes()
}

func encrypt(data []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, data, nil)
	return ciphertext, nil
}

func StartServer(conf config.Config) {
	// Define the server port
	port := "4242"
	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	defer listener.Close()
	fmt.Println("Listening on port", port)

	for {
		// Accept a connection
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}

		go handleRequest(conn, conf)
	}
}

// needs handshake
// needs to crypt data before sending it
func handleRequest(conn net.Conn, conf config.Config) {
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// Loop to keep the connection alive
	for {
		// Read the incoming connection into the buffer.
		message, err := reader.ReadBytes('\n')
		if err != nil {
			fmt.Println("Error reading:", err.Error())
			return
		}
		fmt.Print("Message received: ", string(message))

		ev := CreateEvent(EventData{
			Key:  "key",
			Type: "md",
			Data: []byte("data"),
		}, "12345678901234567890123456789012")

		serializedEvent := SerializeEvent(ev)

		conn.Write(serializedEvent)

		fmt.Printf("Decoded EventData: %+v\n", serializedEvent)

	}
}
