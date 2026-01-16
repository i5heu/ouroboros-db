# OuroborosDB Gemini Assistant Context

This document provides context for the Gemini AI assistant to understand and assist with the OuroborosDB project.

## Project Overview

OuroborosDB is a distributed, content-addressable database library for Go. It is designed to be a stateless-ish and self-healing cluster. Data is addressed by hashes and organized in a Git-like DAG of Blobs.

The project is currently in an early alpha stage and is intended to be embedded into other Go projects.

### Core Data Structures
- **Blobs**: Versioned objects forming a Git-like DAG with parents and heads.
- **Vertices**: The logical units in the DAG containing metadata (parent, timestamp) and references to content (ChunkHashes).
- **Chunks**: Deduplicated content blocks.
- **SealedChunks**: Encrypted and integrity-verified chunks.
- **Blocks**: Archive structures (approx. 16MB) containing multiple SealedChunks and Vertices.
- **SealedSlices**: Compressed, erasure-coded (Reed-Solomon), and encrypted fragments of blocks distributed across nodes.

### Key Features
- **Content-Addressable**: Data is identified by its hash.
- **Post-Quantum Cryptography**: Uses `ouroboros-crypt` for ML-KEM (key encapsulation) and ML-DSA (signatures).
- **QUIC Transport**: Reliable, encrypted communication between nodes.
- **Self-Healing**: Convergence via `SyncIndexTree` (Merkle-style sync) and `DeletionWAL` (tombstone propagation).

## Building and Running

### Dependencies
- Go 1.24.6+
- `github.com/i5heu/ouroboros-crypt`: Cryptographic primitives.
- `github.com/lmittmann/tint`: Structured logging.

### Key Commands
- **Build**: `go build ./...`
- **Test**: `go test ./...`
- **Heavy Tests**: `./testHeavy.sh`
- **Benchmark**: `benchmarkVersions.bash`

## Development Conventions

### Correctness Annotations
The project uses specific annotations in function comments (on the same line as the `func` keyword) to indicate the status of the logic:

- `// A`: Written by AI, not reviewed.
- `// AP`: Written by AI, potential issue found (marked with `// TODO`).
- `// AC`: Written by AI, reviewed and approved with medium confidence.
- `// H`: Written by a human.
- `// HP`: Written by a human, potential issue found.
- `// HC`: Written by a human, high confidence in correctness and safety.

**Priority Functions**: High-risk functions (complex algorithms, security-sensitive) are prefixed with `P` (e.g., `// PHC`). All `P` functions must reach `PHC` status before production release.

## Architecture

The system is composed of several key layers:

1.  **CAS (Content-Addressable Storage)**: The primary entry point for data operations. It coordinates chunking, encryption, WAL buffering, and indexing.
2.  **Carrier**: Manages inter-node communication, cluster membership, and message broadcasting using QUIC and post-quantum crypto.
3.  **DataRouter**: Routes data to the appropriate nodes for storage and retrieval.
4.  **BlockStore**: Low-level storage for blocks and slices.
5.  **WAL (Write-Ahead Log)**: Buffers data (Chunks and Vertices) until enough is collected to seal a Block.

### File Structure Highlights
- `ouroboros.go`: Library entry point.
- `pkg/storage/cas.go`: CAS interface and documentation.
- `pkg/model/`: Core data models (`vertex.go`, `chunk.go`, etc.).
- `internal/carrier/`: Implementation of the communication layer.
- `docs/diagrams/`: Mermaid diagrams of the architecture and sequences.
