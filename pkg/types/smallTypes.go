package types

import (
	"bytes"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/i5heu/ouroboros-db/internal/protoFastMeta"
	"google.golang.org/protobuf/proto"
)

const (
	TrueStr  = "true"
	FalseStr = "false"
)

// EventType defines how the event should behave, for example if i get the File of a Patch event i will get the finishes file with all previous patches applied
// TODO explain in more detail
type EventType int

const (
	Normal EventType = iota
	Overwrite
	Patch
	Root
)

func (e EventType) String() string {
	switch e {
	case Normal:
		return "Normal"
	case Overwrite:
		return "Overwrite"
	case Patch:
		return "Patch"
	case Root:
		return "Root"
	}
	return "Unknown"
}

func (e EventType) Bytes() []byte {
	return []byte{byte(e)}
}

func (e *EventType) FromBytes(b []byte) error {
	if len(b) != 1 {
		return fmt.Errorf("invalid byte length for EventType: %d", len(b))
	}
	*e = EventType(b[0])
	return nil
}

type Hash [64]byte

func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

func (h Hash) Bytes() []byte {
	return h[:]
}

func (h *Hash) HashFromBytes(b []byte) error {
	if len(b) != 64 {
		return fmt.Errorf("invalid byte length for Hash: %d", len(b))
	}
	copy(h[:], b)
	return nil
}

type RootEventsIndex struct {
	Title []byte
	Hash  Hash
}

type Binary bool

func (b Binary) Bytes() []byte {
	if b {
		return []byte(TrueStr)
	}
	return []byte(FalseStr)
}

func (b Binary) String() string {
	if b {
		return TrueStr
	}
	return FalseStr
}

// Title can be used as a category, pseudo type, or name of an event.
// Its primary purpose is to speed up the search for events.
// Its most important use is to have root events with a somewhat unique title,
// so one can find a fast entry point to a desired EventChain.
type Title string

func (t Title) Bytes() []byte {
	return []byte(t)
}

// This is a unix timestamp in nanoseconds
// It is used to have some idea of the time of creation of an event.
// We recognize that the time of computer systems is not reliable, that is why we call it Level, instead of creation time.
type Level int64

func (l Level) Bytes() []byte {
	levelBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(levelBytes, uint64(l))
	return levelBytes
}

func (l Level) String() string {
	return strconv.FormatInt(int64(l), 10)
}

func (l Level) Time() time.Time {
	return time.Unix(0, int64(l))
}

func (l *Level) SetToNow() {
	*l = Level(time.Now().UnixNano())
}

// FastMetaParameter is a single parameter of FastMeta
type FastMetaParameter []byte

func (f FastMetaParameter) String() string {
	return hex.EncodeToString(f)
}

type FastMetaBytesSerialized []byte

// The maximum size of the FastMeta is 128kb
const FastMetaMaxSize = 128 * 1024

// FastMeta allows for metadata to be stored in the description of the event itself, what leads to faster access without indexing.
// FastMeta will be stored in the key and not the value of the underlying key-value store.
type FastMeta []FastMetaParameter

func (f FastMeta) IsToBig() bool {
	var size int
	for _, parameter := range f {
		size += len(parameter)
	}
	return size > FastMetaMaxSize
}

func (f FastMeta) Size() int {
	var size int
	for _, parameter := range f {
		size += len(parameter)
	}
	return size
}

func (f FastMeta) SizeLeft() int {
	return FastMetaMaxSize - f.Size()
}

func (f FastMeta) Hash() Hash {
	// Pre-allocate a buffer to make the hashing process more efficient
	var buffer bytes.Buffer

	if len(f) == 0 {
		return Hash{}
	}

	// Append all FastMetaParameters
	for _, parameter := range f {
		buffer.Write(parameter[:])
		buffer.WriteString(":")
	}

	// Hash the buffer
	hash := sha512.Sum512(buffer.Bytes())

	return hash
}

func (f FastMeta) Bytes() FastMetaBytesSerialized {
	fastProto := protoFastMeta.FastMetaProto{}

	for _, parameter := range f {
		fastProto.Parameters = append(fastProto.Parameters, &protoFastMeta.FastMetaParameterProto{Parameter: parameter})
	}

	buffer, _ := proto.Marshal(&fastProto)

	return buffer
}

func (f *FastMeta) FromBytes(data FastMetaBytesSerialized) error {
	fastProto := protoFastMeta.FastMetaProto{}
	err := proto.Unmarshal(data, &fastProto)
	if err != nil {
		return err
	}

	*f = FastMeta{}
	for _, parameter := range fastProto.Parameters {
		*f = append(*f, FastMetaParameter(parameter.Parameter))
	}

	return nil
}

type ChunkMeta struct {
	Hash       Hash   // SHA-512 hash
	DataLength uint32 // The length of the data chunk
}

type Chunk struct {
	ChunkMeta
	Data ECCCollection
}

type ECCMetaCollection []ECCMeta
type ECCMeta struct {
	ECChunkHash    Hash
	OtherECCHashes []Hash

	BuzChunkNumber uint32
	BuzChunkHash   Hash

	ECCNumber           uint8
	ECC_k               uint8
	ECC_n               uint8
	EncryptedDataLength uint32
}

type ECCCollection []ECChunk

type ECChunk struct {
	ECCMeta
	EncryptedData []byte
}
