# OuroborosDB Gemini Assistant Context

This document provides context for the Gemini AI assistant to understand and assist with the OuroborosDB project.

## Project Overview

OuroborosDB is a distributed, content-addressable database library for Go. It is designed to be a stateless-ish and self-healing cluster. Data is addressed by hashes and organized in a Git-like DAG of Blobs.

The core data structures are:

*   **Blobs**: Versioned objects, forming a Git-like DAG with parents and heads.
*   **Chunks**: Deduplicated content blocks.
*   **SealedSlices**: Compressed, erasure-coded, and encrypted fragments distributed across nodes.

The cluster relies on a `SyncIndexTree` (Merkle-style sync) and a `DeletionWAL` (tombstone propagation) to keep replicas convergent without a global consensus log.

OuroborosDB is intended to be embedded into other Go projects as a single root package.

## Building and Running

### Dependencies

The project uses Go modules for dependency management. The main dependencies are:

*   `github.com/dgraph-io/badger/v4`: A fast key-value DB in Go.
*   `github.com/i5heu/ouroboros-crypt`: Cryptographic primitives for OuroborosDB.
*   `github.com/klauspost/compress`: Compression libraries for Go.
*   `github.com/klauspost/reedsolomon`: Reed-Solomon erasure coding in Go.
*   `google.golang.org/protobuf`: Protocol Buffers for Go.

### Building

To build the project, use the standard Go build command:

```bash
go build ./...
```

### Testing

The project has a suite of tests that can be run with the following command:

```bash
go test ./...
```

There is also a "heavy" test suite that can be run with:

```bash
./testHeavy.sh
```

## Development Conventions

The project uses a system of annotations in function comments to indicate the correctness and safety of the logic. These annotations are:

*   `// A`: Written by AI, not reviewed.
*   `// AP`: Written by AI, potential issue found.
*   `// AC`: Written by AI, reviewed and approved with medium confidence.
*   `// H`: Written by a human.
*   `// HP`: Written by a human, potential issue found.
*   `// HC`: Written by a human, high confidence in correctness.

A `P` prefix can be added for high-priority functions that must be brought to `PHC` status before a production release.

## Architecture

The project's architecture is detailed in `docs/diagrams/architecture.mmd`. The main components are:

*   **Cluster**: A collection of nodes.
*   **Node**: A single member of the cluster.
*   **Carrier**: Handles communication between nodes.
*   **DataRouter**: Routes data to the correct node for storage or retrieval.
*   **CAS (Content Addressable Storage)**: Stores and retrieves blobs by their hash.
*   **LocalSealedSliceStore**: A local key-value store for sealed slices.
*   **DistributedIndex**: A distributed index for mapping hashes to nodes and keys to hashes.
*   **DataReBalancer**: Rebalances data across the cluster.
*   **SyncIndexTree**: A Merkle-style tree for synchronizing data between nodes.
*   **DeletionWAL**: A write-ahead log for propagating deletions.
