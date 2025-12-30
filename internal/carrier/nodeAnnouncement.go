package carrier

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// NodeAnnouncement contains node information for cluster-wide propagation.
// This is serialized and sent as the payload of MessageTypeNewNodeAnnouncement.
type NodeAnnouncement struct { // A
	// Node contains the announced node's information.
	Node Node
	// Timestamp is when this announcement was created (Unix nanoseconds).
	Timestamp int64
}

// SerializeNodeAnnouncement encodes a NodeAnnouncement to bytes.
// Format: [8 bytes timestamp][4 bytes nodeID len][nodeID][4 bytes addr count]
//
//	[for each addr: 4 bytes len, addr bytes][4 bytes pubkey len][pubkey bytes]
func SerializeNodeAnnouncement(ann *NodeAnnouncement) ([]byte, error) { // A
	if ann == nil {
		return nil, errors.New("nil announcement")
	}

	// Calculate total size
	nodeIDBytes := []byte(ann.Node.NodeID)
	size := 8 + // timestamp
		4 + len(nodeIDBytes) + // nodeID
		4 // address count

	for _, addr := range ann.Node.Addresses {
		size += 4 + len(addr) // each address
	}

	// For now, we skip public key serialization as it's complex
	// The public key can be exchanged during the connection handshake
	size += 4 // pubkey length (will be 0)

	// Build the buffer
	buf := make([]byte, size)
	offset := 0

	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(ann.Timestamp))
	offset += 8

	// NodeID
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(nodeIDBytes)))
	offset += 4
	copy(buf[offset:], nodeIDBytes)
	offset += len(nodeIDBytes)

	// Addresses
	binary.BigEndian.PutUint32(buf[offset:], uint32(len(ann.Node.Addresses)))
	offset += 4
	for _, addr := range ann.Node.Addresses {
		addrBytes := []byte(addr)
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(addrBytes)))
		offset += 4
		copy(buf[offset:], addrBytes)
		offset += len(addrBytes)
	}

	// Public key length = 0 (not serialized)
	binary.BigEndian.PutUint32(buf[offset:], 0)

	return buf, nil
}

// DeserializeNodeAnnouncement decodes a NodeAnnouncement from bytes.
func DeserializeNodeAnnouncement(data []byte) (*NodeAnnouncement, error) { // A
	if len(
		data,
	) < 16 { // minimum: timestamp + nodeID len + addr count + pubkey len
		return nil, errors.New("data too short for node announcement")
	}

	offset := 0
	ann := &NodeAnnouncement{}

	// Timestamp
	ann.Timestamp = int64(binary.BigEndian.Uint64(data[offset:]))
	offset += 8

	// NodeID
	nodeIDLen := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4
	if offset+nodeIDLen > len(data) {
		return nil, errors.New("invalid nodeID length")
	}
	ann.Node.NodeID = NodeID(data[offset : offset+nodeIDLen])
	offset += nodeIDLen

	// Addresses
	if offset+4 > len(data) {
		return nil, errors.New("data too short for address count")
	}
	addrCount := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4

	ann.Node.Addresses = make([]string, addrCount)
	for i := 0; i < addrCount; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("data too short for address length")
		}
		addrLen := int(binary.BigEndian.Uint32(data[offset:]))
		offset += 4
		if offset+addrLen > len(data) {
			return nil, errors.New("invalid address length")
		}
		ann.Node.Addresses[i] = string(data[offset : offset+addrLen])
		offset += addrLen
	}

	// Public key
	if offset+4 > len(data) {
		return nil, errors.New("data too short for pubkey length")
	}
	pubKeyLen := int(binary.BigEndian.Uint32(data[offset:]))
	offset += 4
	if pubKeyLen > 0 {
		if offset+pubKeyLen > len(data) {
			return nil, errors.New("invalid pubkey length")
		}
		// Note: PublicKey unmarshaling would need to be implemented
		// For now, we skip this as it requires the keys package integration
		_ = data[offset : offset+pubKeyLen]
		// ann.Node.PublicKey = ... unmarshal
	}

	return ann, nil
}

// SerializeNodeList encodes a list of nodes for sync purposes.
// Used for responding to node list requests.
func SerializeNodeList(nodes []Node) ([]byte, error) { // A
	// Calculate size: 4 bytes count + each node serialized
	size := 4
	nodeData := make([][]byte, len(nodes))

	for i, node := range nodes {
		ann := &NodeAnnouncement{Node: node, Timestamp: 0}
		data, err := SerializeNodeAnnouncement(ann)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to serialize node %s: %w",
				node.NodeID,
				err,
			)
		}
		nodeData[i] = data
		size += 4 + len(data) // length prefix + data
	}

	buf := make([]byte, size)
	offset := 0

	binary.BigEndian.PutUint32(buf[offset:], uint32(len(nodes)))
	offset += 4

	for _, data := range nodeData {
		binary.BigEndian.PutUint32(buf[offset:], uint32(len(data)))
		offset += 4
		copy(buf[offset:], data)
		offset += len(data)
	}

	return buf, nil
}

// DeserializeNodeList decodes a list of nodes.
func DeserializeNodeList(data []byte) ([]Node, error) { // A
	if len(data) < 4 {
		return nil, errors.New("data too short for node list")
	}

	nodeCount := int(binary.BigEndian.Uint32(data[0:4]))
	offset := 4

	nodes := make([]Node, 0, nodeCount)
	for i := 0; i < nodeCount; i++ {
		if offset+4 > len(data) {
			return nil, errors.New("data too short for node data length")
		}
		nodeDataLen := int(binary.BigEndian.Uint32(data[offset:]))
		offset += 4

		if offset+nodeDataLen > len(data) {
			return nil, errors.New("invalid node data length")
		}

		ann, err := DeserializeNodeAnnouncement(data[offset : offset+nodeDataLen])
		if err != nil {
			return nil, fmt.Errorf("failed to deserialize node %d: %w", i, err)
		}
		nodes = append(nodes, ann.Node)
		offset += nodeDataLen
	}

	return nodes, nil
}
