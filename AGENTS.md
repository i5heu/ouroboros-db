# AGENTS.md — OuroborosDB Developer Guidelines

## Overview

OuroborosDB is a Go content-addressable distributed database using a Git-like DAG structure.
The data pipeline transforms cleartext through encryption, buffering, batching, and erasure coding:

```
Chunk (cleartext)
  → SealedChunk (encrypted with SealedHash)
  → DistributedWAL (intake buffer, ~16MB batches)
  → Block (DataSection + VertexSection + KeyRegistry + Indexes)
  → BlockSlice (Reed-Solomon shard, distributed to cluster nodes)
```

- Root package: `ouroboros` (see `ouroboros.go`, `config.go`)
- Stable public helpers: `pkg/`
- Internal services: `internal/`
- Binaries/daemons: `cmd/`

**Core principle**: Content addressing with DAG (Directed Acyclic Graph) where everything is identified by cryptographic hashes.

## Design Principles

1. **Masterless AP**: No single point of failure, eventual consistency, high availability
2. **Idempotency**: Operations are safely retryable (stateless-ish nodes)
3. **Content Addressing**: Everything identified by cryptographic hash (Git-like DAG via Vertices)
4. **Encryption Granularity**: Individual chunks encrypted with per-user key wrappers

## Architecture

Read `README.md` and `docs/diagrams/architecture.mmd` for the full picture.

**Important files**: `ouroboros.go`, `config.go` (note `Config.Paths[0]` usage), `internal/logging/clusterlog` (logging).

### Key Data Structures

- **Vertex**: DAG node with metadata, parent references, and ChunkHash pointers (stored UNENCRYPTED in Block's VertexSection)
- **Chunk**: Temporary cleartext content with hash and size
- **SealedChunk**: Encrypted content with Nonce, OriginalSize, and SealedHash (integrity check without decryption)
- **Block**: Central archive unit (~16MB) containing BlockHeader, DataSection (encrypted SealedChunks), VertexSection (unencrypted Vertices), ChunkRegion/VertexRegion indexes, and KeyRegistry (map[Hash][]KeyEntry)
- **KeyEntry**: Per-user encryption key wrapper linking pubKey to chunkHash
- **BlockSlice**: RS-encoded shard with reconstruction params, distributed across nodes

## Quick Commands

```bash
go build ./...
go test ./...
go test -race -count=1 ./...
go test ./pkg/cas -run TestNew    # specific package + test
./coverage.sh                     # coverage report
./testHeavy.sh                    # race + repeated + bench + property (takes time)
golangci-lint-v2 run                 # lint
golangci-lint-v2 run --fix           # lint + auto-format
go mod tidy
```

## Function Annotations (Mandatory)

Every function MUST have an annotation on the same line as `func`:

- `// A` — AI-authored, no review
- `// AP` — AI-authored, reviewed with TODO
- `// AC` — AI-authored, reviewed and approved
- `// H` — Human-authored
- `// HP` — Human-authored with TODO
- `// HC` — Human-comprehended and confident

Production-critical functions prefix with `P`: `// PAP`, `// PAC`, `// PHC`.
All `P` functions must reach `// PHC` before production.

Multiline parameters: annotation goes on the `func` line, not the closing paren.

```go
func exampleFunction() { // A
    // Low-risk, AI-authored
}

func criticalFunction() { // PHC
    // High-risk, production-ready
}
```

## Test File Convention

AI-generated test files use `_A_test.go` suffix (e.g. `cluster_log_A_test.go`).
Humans should NOT add tests in `_A` files — create new files without the suffix.

## Project Structure

```
ouroboros.go            # Root type + constructor New(Config)
config.go               # Config, StorageConfig, NetworkConfig, IdentityConfig
pkg/
  interfaces/           # All interface definitions (Carrier, Storage, etc.)
  authfile/             # Auth file handling
internal/
  auth/                 # Auth system: CA, certificates, delegation, canonical
    ca/                 # Admin/User CA logic
    cert/               # Node identity/cert management
    delegation/         # Delegation proofs, session identity
    canonical/          # Canonical encoding for verification
  control/              # ClusterController
  datacas/cas/          # CAS primitives + privilege checker
  entities/node/        # Node entity
  logging/
    clusterlog/         # Cluster-wide logging (USE THIS for all logging)
    logtypes/           # LogLevel, LogEntry types
  transport/            # QUIC carrier, auth handshake, heartbeat, bootstrap
proto/                  # Generated protobuf code (DO NOT EDIT)
  auth/                 # Canonical protobuf
  authfile/             # Authfile protobuf
  carrier/              # Message, heartbeat, auth handshake protobufs
cmd/
  start/                # Main daemon entrypoint
  certgen/              # Certificate generation tool
  interactive/          # Interactive CLI
tests/                  # Integration/compliance tests (outside packages)
benchmark/              # Docker-based benchmarking tools
docs/diagrams/          # Architecture diagrams
```

## Code Style

- **Line length**: 100 chars max (enforced by `lll` and `golines`); unlimited in test files
- **Cyclomatic complexity**: max 10 (`cyclop`)
- **Formatters**: `gofumpt`, `goimports`, `gci`, `golines`, `gofmt` (via `golangci-lint-v2 run --fix`)
- **Imports grouped**: stdlib → third-party → local
- **Test naming**: `TestXxx`, `BenchmarkXxx`, `ExampleXxx`
- **Table-driven tests** preferred
- **Property tests**: `pgregory.net/rapid` (uses fork: `replace pgregory.net/rapid => github.com/flyingmutant/rapid v1.2.0` in go.mod)

## Logging Policy

**Always use `internal/logging/clusterlog`** for logging.
Exception only when it would create subscription loops — document with `// LOGGER` comment.

**sloglint rules** (enforced by linter):
- kv-only (no mixed args, no attributes)
- context-aware methods (`context` must be `"all"`)
- Static messages only
- camelCase keys, no raw keys
- Lowercased messages
- No global loggers

## Testing Requirements

- Coverage: file 50%, package 80%, total 85% minimum — enforced in CI via `.github/.testcoverage.yml`
- Tests in `_test.go` files alongside code
- Integration/compliance tests in `tests/` at repo root
- Set `RAPID_CHECKS=10_000` for thorough property testing

```bash
./coverage.sh    # runs tests + prints per-function coverage
```

## Reference Files

- `README.md` — Annotation legend, hardware constraints, development focus
- `docs/diagrams/architecture.mmd` — Architecture diagram (457 lines, comprehensive reference)
- `.golangci.yml` — Linting/formatting rules
- `.github/.testcoverage.yml` — CI coverage configuration
