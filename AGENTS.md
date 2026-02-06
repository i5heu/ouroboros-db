# AGENTS.md — OuroborosDB Developer Guide

Guidelines for AI coding agents working on OuroborosDB, a Go-based content-addressable distributed database.

## Build, Test, and Lint Commands

### Essential Commands
```bash
# Build all packages
go build ./...

# Run all tests
go test ./...

# Run specific test (e.g., TestNew)
go test ./... -run TestNew

# Run with race detector
go test -race -count=1 ./...

# Generate coverage report
go test ./... -coverprofile=./cover.out -covermode=atomic -coverpkg=./...

# Format code
go fmt ./...

# Lint (golangci-lint)
golangci-lint run

# Format and fix lint issues
golangci-lint run --fix

# Tidy dependencies
go mod tidy
```

### Heavy Testing (comprehensive)
Use `./testHeavy.sh` for thorough validation:
```bash
./testHeavy.sh  # Runs race detection, repeated tests, benchmarks, and property tests
```

## Critical Convention: Function Annotations

**BLOCKING REQUIREMENT**: Every function MUST have an authorship/review annotation on the same line as `func`:

- `// A` — AI-authored, no human review
- `// AP` — AI-authored, human reviewed with TODO
- `// AC` — AI-authored, human reviewed and approved
- `// H` — Human-authored
- `// HP` — Human-authored with TODO
- `// HC` — Human comprehended and confident

For production-critical functions, prefix with `P`:
- `// PAP`, `// PAC`, `// PHC` — must reach `// PHC` before production release

Example:
```go
func New(conf Config) (*OuroborosDB, error) { // A
    // implementation
}

func criticalOperation() { // PHC
    // production-ready, human-comprehended
}
```

## Code Style Guidelines

### Imports
- Use standard library first, then third-party, then internal
- Group imports with blank lines between groups
- Use `goimports` or `go fmt` for automatic formatting

### Naming Conventions
- **Exported types/functions**: PascalCase (e.g., `OuroborosDB`, `New`)
- **Unexported**: camelCase (e.g., `defaultLogger`)
- **Interfaces**: `-er` suffix (e.g., `Reader`, `Writer`)
- **Test files**: `*_test.go` alongside source files
- **Test functions**: `TestXxx`, `BenchmarkXxx`, `ExampleXxx`

### Error Handling
- Return errors as the last return value
- Wrap errors with context using `fmt.Errorf("...: %w", err)`
- Check errors immediately: `if err != nil { return err }`
- Use `log/slog` for structured logging, never panic in library code

### Types and Structs
- Prefer concrete types over interfaces for data structures
- Use embedding for composition
- Document exported types with comments starting with the type name

### Testing
- **Coverage Gate**: 50% minimum (files, packages, total)
- Use table-driven tests
- Property testing with `pgregory.net/rapid` (set `RAPID_CHECKS` env var)
- Place test utilities in `internal/testutil`

## Project Structure

- **Root package**: `ouroboros` — main library code (`ouroboros.go`, `config.go`)
- **cmd/**: CLI and daemon binaries
- **pkg/**: Public API packages (stable)
- **internal/**: Implementation details (may change)

## Architecture Notes

- **Content addressing**: Everything identified by cryptographic hash
- **DAG structure**: Git-like directed acyclic graph via Vertices
- **Idempotency**: Design operations to be safely retryable
- **Encryption**: Per-chunk with key wrappers
- **Replication**: Reed-Solomon encoded BlockSlices distributed across nodes

## Key Dependencies

- `github.com/i5heu/ouroboros-crypt` — encryption/sealing
- `github.com/dgraph-io/badger/v4` — embedded KV store
- `github.com/klauspost/reedsolomon` — erasure coding
- `pgregory.net/rapid` — property-based testing

## References

- `CLAUDE.md` — comprehensive architecture documentation
- `.github/copilot-instructions.md` — detailed AI developer instructions
- `docs/diagrams/architecture.mmd` — Mermaid class diagram
- `README.md` — full annotation legend and project overview
