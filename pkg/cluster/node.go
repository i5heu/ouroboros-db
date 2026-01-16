// Package cluster defines interfaces and types for cluster management in OuroborosDB.
//
// This package provides the foundational types for cluster membership and
// node identity. It defines how nodes are identified, authenticated, and
// organized within an OuroborosDB cluster.
//
// # Cluster Architecture
//
// An OuroborosDB cluster consists of multiple nodes that cooperate to
// provide distributed storage:
//
//	┌──────────────────── Cluster ────────────────────┐
//	│                                                 │
//	│   ┌─────────┐   ┌─────────┐   ┌─────────┐      │
//	│   │  Node   │   │  Node   │   │  Node   │      │
//	│   │ (ID: A) │   │ (ID: B) │   │ (ID: C) │      │
//	│   └─────────┘   └─────────┘   └─────────┘      │
//	│        │             │             │           │
//	│        └─────────────┼─────────────┘           │
//	│                      │                         │
//	│              ┌───────┴───────┐                 │
//	│              │    Carrier    │                 │
//	│              │ (QUIC + P2P)  │                 │
//	│              └───────────────┘                 │
//	└─────────────────────────────────────────────────┘
//
// # Node Identity
//
// Each node has a cryptographic identity consisting of:
//   - Public/private key pair (ML-KEM for encryption, ML-DSA for signing)
//   - NodeID: hash of public key (globally unique)
//   - NodeCert: signed certificate for authentication
//
// This provides:
//   - Strong node authentication
//   - End-to-end encryption between nodes
//   - Post-quantum security
//
// # Node Discovery
//
// Nodes discover each other through:
//   - Bootstrap addresses (initial cluster join)
//   - Node announcements (propagated via Carrier)
//   - Heartbeats (liveness and discovery)
package cluster

import (
	"github.com/i5heu/ouroboros-crypt/pkg/hash"
	"github.com/i5heu/ouroboros-crypt/pkg/keys"
)

// NodeID is a unique identifier for a node in the cluster.
//
// NodeID is derived from the hash of the node's public key, ensuring
// global uniqueness and cryptographic binding to the node's identity.
// Two nodes with different key pairs will always have different NodeIDs.
//
// The string representation is typically the hex encoding of the hash.
type NodeID string

// NodeCert represents the certificate used to authenticate a node.
//
// NodeCert provides proof of node identity. It contains the node's public
// key and a self-signature that proves ownership of the corresponding
// private key. Other nodes verify this signature before trusting the identity.
//
// # Verification
//
// To verify a NodeCert:
//  1. Check that PubKeyHash equals hash(PublicKey)
//  2. Verify Signature over PublicKey using the PublicKey itself
//
// This proves the sender knows the private key corresponding to PublicKey.
type NodeCert struct {
	// PubKeyHash is the hash of the node's public key.
	// This should equal hash(PublicKey) and serves as a compact identifier.
	PubKeyHash hash.Hash

	// PublicKey is the node's public key for encryption and verification.
	// Used for:
	//   - Encrypting messages to this node (ML-KEM)
	//   - Verifying signatures from this node (ML-DSA)
	PublicKey keys.PublicKey

	// Signature is a self-signature over the public key (for integrity).
	// Created by signing hash(PublicKey) with the node's private key.
	Signature []byte
}

// Node represents a node in the OuroborosDB cluster.
//
// Node contains all information needed to communicate with a cluster member:
// its identity (NodeID, Cert) and network location (Addresses).
//
// # Addresses
//
// A node may have multiple addresses for:
//   - Multiple network interfaces
//   - IPv4 and IPv6 support
//   - Different ports for different purposes
//
// The Carrier will try addresses in order until one succeeds.
type Node struct {
	// NodeID is the unique identifier for this node.
	// Derived from hash(PublicKey) and immutable for the node's lifetime.
	NodeID NodeID

	// Addresses are the network addresses where this node can be reached.
	// Format: "host:port" (e.g., "192.168.1.10:4433", "[::1]:4433")
	// For QUIC transport, these are UDP addresses.
	Addresses []string

	// Cert is the node's certificate for authentication.
	// Used to verify the node's identity when establishing connections.
	Cert NodeCert
}

// NodeIdentity defines the interface for a node's cryptographic identity.
//
// NodeIdentity encapsulates the cryptographic operations a node can perform
// with its key pair. This includes signing, verification, encryption, and
// decryption operations used for secure inter-node communication.
//
// # Key Management
//
// Implementations typically:
//   - Generate keys on first run (NewNodeIdentity)
//   - Load keys from file on subsequent runs (LoadFromFile)
//   - Store keys securely (SaveToFile with proper permissions)
//
// # Thread Safety
//
// Implementations must be safe for concurrent use.
type NodeIdentity interface {
	// GetPublicKey returns the node's public key.
	//
	// This is used to create the NodeCert and for other nodes to
	// encrypt messages to this node.
	GetPublicKey() *keys.PublicKey

	// Sign signs data using the node's private key (ML-DSA).
	//
	// Parameters:
	//   - data: The bytes to sign
	//
	// Returns:
	//   - The signature bytes
	//   - Error if signing fails
	//
	// Other nodes verify this signature using GetPublicKey().
	Sign(data []byte) ([]byte, error)

	// Verify verifies a signature using the node's public key.
	//
	// Parameters:
	//   - data: The original signed bytes
	//   - signature: The signature to verify
	//
	// Returns:
	//   - true if the signature is valid
	//   - false if verification fails
	//
	// Note: This verifies signatures made by THIS node. To verify
	// another node's signature, use their PublicKey directly.
	Verify(data, signature []byte) bool

	// EncryptFor encrypts data for a specific recipient using their public key.
	//
	// This uses ML-KEM for post-quantum secure key encapsulation,
	// combined with AES for symmetric encryption.
	//
	// Parameters:
	//   - data: The plaintext to encrypt
	//   - recipientPub: The recipient's public key
	//
	// Returns:
	//   - The encrypted data (ciphertext + encapsulated key)
	//   - Error if encryption fails
	EncryptFor(data []byte, recipientPub *keys.PublicKey) ([]byte, error)

	// Decrypt decrypts data that was encrypted for this node.
	//
	// This uses the node's private key to decapsulate the symmetric key,
	// then decrypts the ciphertext.
	//
	// Parameters:
	//   - encryptedData: Data encrypted with EncryptFor(data, thisNode.PublicKey)
	//
	// Returns:
	//   - The decrypted plaintext
	//   - Error if decryption fails (wrong key, corrupted data)
	Decrypt(encryptedData []byte) ([]byte, error)

	// SaveToFile persists the node identity to a key file.
	//
	// The file should be stored securely with restricted permissions
	// (e.g., 0600) as it contains the private key.
	//
	// Parameters:
	//   - filepath: Path to save the key file
	//
	// Returns:
	//   - Error if the file cannot be written
	SaveToFile(filepath string) error
}
