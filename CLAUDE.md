# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

OuroborosDB is a content-addressable distributed database library written in Go, currently in early development (v0.1.1-alpha-3, not production-ready). The architecture centers on a Git-like DAG structure with encryption at the chunk level, erasure coding for resilience, and distributed replication across cluster nodes.

**Core principle**: Content addressing with DAG (Directed Acyclic Graph) structure where everything is identified by cryptographic hashes.

## Build and Test Commands

### Standard Development
```bash
# Build all packages
go build ./...

# Run fast tests
go test ./...

# Run with race detector
go test -race -count=1 ./...

# Generate coverage report
go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...
```

### Heavy Testing
Use `./testHeavy.sh` for comprehensive testing:
- Race detection: `go test -race -count=1 ./...`
- Repeated runs (10x): `go test -count=10 -p=5 ./...`
- Benchmarks: `go test -bench=. -benchtime=10s ./...`
- Property testing: `RAPID_CHECKS=10_000 go test -count=1 -v ./...`

### Benchmarks
```bash
# Run benchmarks
go test -bench=. -benchtime=10s ./...

# Dockerized version comparison (writes to docs/bench-html/)
./benchmarkVersions.bash
```

### Maintenance
```bash
# Tidy dependencies (resolve current diagnostics)
go mod tidy

# Format code
go fmt ./...

# Lint code
golangci-lint run

# Lint and auto-fix issues
golangci-lint run --fix
```

### Running Specific Tests
```bash
# Run specific test by name
go test ./... -run TestNew

# Run tests for specific package
go test ./pkg/cas/...

# Run specific test in specific package
go test ./pkg/cas -run TestNew
```

## Critical Development Convention: Function Annotations

**BLOCKING REQUIREMENT**: All functions MUST have authorship/review annotations immediately after `func` declaration.

Append one of these annotations at the same line as `func`:
- `// A` - AI-authored, no human review
- `// AP` - AI-authored, human reviewed with TODO
- `// AC` - AI-authored, human reviewed and approved
- `// H` - Human-authored
- `// HP` - Human-authored with TODO
- `// HC` - Human comprehended and confident

For production-critical functions, prefix with `P`:
- `// PAP`, `// PAC`, `// PHC` - All P-prefixed functions must reach `// PHC` before production release

Examples:
```go
// This function does X, Y, and Z.
func exampleFunction() { // A
    // Low-risk function, AI-authored, not reviewed
}

func manyParametersFunction( // AC
    param1 string,
    param2 int,
) error {
    // Annotation goes at same line as func keyword
    return nil
}

// Critical security operation
func criticalFunction() { // PHC
    // High-risk function, human-comprehended and production-ready
}
```

## Architecture Overview

### Data Model: Content Pipeline

The core data flow follows a transformation pipeline from cleartext to distributed shards:

```
Chunk (cleartext)
  ↓ [encryption via EncryptionService]
SealedChunk (encrypted with SealedHash for integrity)
  ↓ [buffering in DistributedWAL]
DistributedWAL (intake buffer, aggregates until ~16MB)
  ↓ [batching via SealBlock()]
Block (DataSection + VertexSection + KeyRegistry + Indexes)
  ↓ [Reed-Solomon erasure coding]
BlockSlice (physical shard distributed to cluster nodes)
```

**Key Data Structures**:

- **Vertex** (formerly "Blob"): DAG node with metadata, parent references, and ChunkHash pointers (stored UNENCRYPTED in Block's VertexSection)
- **Chunk**: Temporary cleartext content with hash and size
- **SealedChunk**: Encrypted content with Nonce, OriginalSize, and SealedHash (integrity check without decryption)
- **Block**: Central archive unit (~16MB) containing:
  - BlockHeader (metadata, Reed-Solomon params)
  - DataSection (encrypted SealedChunks)
  - VertexSection (unencrypted Vertices for DAG structure)
  - ChunkRegion Index (ChunkHash → byte offset/length)
  - VertexRegion Index (VertexHash → byte offset/length)
  - KeyRegistry (map[Hash][]KeyEntry for access control)
- **KeyEntry**: Per-user encryption key wrapper linking pubKey to chunkHash
- **BlockSlice**: RS-encoded shard with reconstruction params, distributed across nodes

### Core Components

**DataRouter**: Orchestrates data operations across cluster
- Routes `StoreVertex()`, `RetrieveVertex()`, `DeleteVertex()`
- Coordinates `DistributeBlockSlices()` to nodes
- Works with BlockDistributionTracker for slice placement

**CAS (Content-Addressable Storage)**: High-level API
- User-facing: `StoreContent()`, `GetContent()`, `DeleteContent()`
- DAG operations: `GetVertex()`, `ListChildren()`
- Internally uses DistributedWAL, BlockStore, EncryptionService

**BlockStore**: Low-level block persistence
- Store/retrieve full blocks and individual slices
- Region-based retrieval: `GetSealedChunkByRegion()`, `GetVertexByRegion()` for partial reads
- Manages `StoreBlockSlice()`, `GetBlockSlice()`, `ListBlockSlices()`

**EncryptionService**: Encryption layer using ouroboros-crypt
- `SealChunk()`: Chunk → SealedChunk + KeyEntry
- `UnsealChunk()`: SealedChunk + KeyEntry + private key → Chunk
- `GenerateKeyEntry()`: Creates per-user key wrappers

**BlockDistributionTracker**: Tracks slice distribution state
- `StartDistribution()`, `RecordSliceConfirmation()`, `GetDistributionState()`
- `MarkDistributed()`, `MarkFailed()` for finalization
- Enables offline node synchronization via metadata queries

### Clustering and Replication

**Node Architecture**:
- Each Node runs CAS locally, maintains Index, persists via BlockStore
- Nodes are mostly stateless; operations designed to be idempotent for replay
- Logs emitted to ClusterLog for cluster-wide observability

**Cluster Components**:
- **Carrier**: Network transport abstraction (`GetNodes()`, `Broadcast()`, `SendMessageToNode()`, `JoinCluster()`)
- **ClusterController**: Per-node control layer, listens on Carrier, coordinates DataRouter
- **DataReBalancer**: Orchestrates slice rebalancing using ReplicationMonitoring and SyncIndexTree
- **ClusterMonitor**: Cluster-wide health and observability (`MonitorNodeHealth()`, `CollectClusterLogs()`, `CollectNodeStats()`)

**Observability**:
- **ClusterLog**: Centralized log aggregation with `Append()`, `Tail()`, `Query()`, `QueryByLevel()`
- **DataState**: Cluster-wide data placement view (`GetNodesForVertex()`, `GetNodesForBlock()`, NodeDataStatus: Present/Missing/Syncing/PendingDelete/Corrupt)
- **NodeStats**: Per-node counters (VertexCount, BlockCount, SealedChunkCount, etc.) for capacity-aware placement

**Consistency Mechanisms**:
- **SyncIndexTree**: Distributed index synchronization, ensures nodes converge on shared state
- **DeletionWAL**: Deletion Write-Ahead Log for safe, replicated garbage collection
- **Indexes**: parentChildIndex (DAG relationships), VersionIndex (multi-head version vectors), KeyToHashIndex (key→hash mapping)

**Replication Strategy**:
- Data stored as Reed-Solomon encoded BlockSlices
- Distributed across cluster via DataRouter + BlockDistributionTracker
- Confirmation tracking enables detection of replication gaps
- Idempotent operations allow safe retry and recovery

## Project Structure

**Root Package** (`github.com/i5heu/ouroboros-db`):
- `ouroboros.go`: Root type `OuroborosDB` and constructor `New(conf Config)`
- `config.go`: `Config` struct (Paths, Logger, UiPort)
  - Note: Only `Paths[0]` is currently used

**Planned Structure** (early-stage, mostly unimplemented):
- `pkg/` - Public packages: `pkg/cas` (CAS primitives), `pkg/logging` (slog wrapper)
- `internal/` - Internal services: `api`, `cluster`, `dataRouting`, `health`, `index`, `node`, `rebalance`, `transport`, `testutil`
- `cmd/cli`, `cmd/daemon` - Entry points for tooling
- `benchmark/` - Docker-based benchmarking with results in `docs/bench-html/`

## Key External Dependencies

- `github.com/i5heu/ouroboros-crypt` - External crypto module for encryption/sealing (check this for crypto patterns)
- `github.com/dgraph-io/badger/v4` - Embedded KV store for local persistence
- `github.com/klauspost/reedsolomon` - Reed-Solomon erasure coding
- `github.com/ipfs/boxo` - IPFS utilities for DAG/content addressing
- `google.golang.org/protobuf` - Serialization

## Code Placement Guidelines

- **Library changes**: Use root `ouroboros` package (`ouroboros.go`, `config.go`)
- **New binaries**: Add under `cmd/<binary>/`
- **Public APIs**: Add to `pkg/` when stability is required
- **Internal services**: Add to `internal/` for implementation details that may change

## Design Principles

1. **Idempotency**: Design operations to be safely retryable (stateless-ish nodes)
2. **Content Addressing**: Everything identified by cryptographic hash (Git-like DAG via Vertices)
3. **Encryption Granularity**: Individual chunks encrypted with per-user key wrappers
4. **Convergence**: Use SyncIndexTree for index synchronization across cluster
5. **Safe Deletion**: Use DeletionWAL for replicated garbage collection

## Testing Requirements

- **Coverage Gate**: 50% minimum (files, packages, total) enforced by `.github/.testcoverage.yml`
- **Test Location**: Tests live in `_test.go` files alongside code
- **Test Naming**: `TestXxx` for tests, `BenchmarkXxx` for benchmarks, `ExampleXxx` for examples
- **Table-Driven Tests**: Preferred pattern
- **Property Testing**: Use `pgregory.net/rapid` for property-based tests (set `RAPID_CHECKS` env var)
- **Helpers**: Use `internal/testutil` for test utilities

## Logging Policy

**All logging in this repository must use the `pkg/clusterlog` package.** This ensures logs are recorded in the cluster-wide log, emitted to the configured `slog.Logger`, and delivered to subscribers via the Carrier transport where applicable.

**Exception**: Only use direct `slog.Logger` when using `pkg/clusterlog` would create or risk a subscription loop (circular subscriptions) that could cause self-delivery or unbounded propagation. In such cases, document the justification with a clear `// LOGGER` comment referencing the potential subscription loop.

## Linting and Code Quality

This project uses `golangci-lint` with strict rules enforced via `.golangci.yml`:

**Linters**:
- `govet`, `errcheck`, `staticcheck`, `unused`, `ineffassign` - Standard Go quality checks
- `gosec` - Security inspection
- `sloglint` - Structured logging best practices (kv-only, context-aware, static messages, camelCase keys, lowercased messages, no raw keys)
- `cyclop` - Cyclomatic complexity (max 10)
- `lll` - Long line length (max 80 chars, tab-width 2)

**Formatters** (auto-applied with `--fix`):
- `gofumpt` - Stricter gofmt with extra rules
- `goimports` - Format and fix imports
- `gci` - Group imports (stdlib → third-party → local)
- `golines` - Shorten long lines (80 char max, 2 tab width)
- `gofmt` - Standard Go formatting

## Example Usage

```go
package main

import "github.com/i5heu/ouroboros-db"

func main() {
    conf := ouroboros.Config{
        Paths: []string{"/tmp/ouroboros"}, // Only Paths[0] is used
    }
    db, err := ouroboros.New(conf)
    if err != nil {
        // handle error
    }
    // Use db...
}
```

## Important Reference Files

- `docs/diagrams/architecture.mmd` - Complete Mermaid class diagram (457 lines, comprehensive architectural reference)
- `README.md` - Development notes and full annotation legend
- `AGENTS.md` - Repository guidelines for contributors
- `.github/copilot-instructions.md` - Detailed AI developer instructions
