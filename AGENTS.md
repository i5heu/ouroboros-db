# AGENTS.md — OuroborosDB Developer Guidelines

## Communication Guidelines

**Always use the `question` tool** to ask users for clarification or decisions before taking actions. Do not make assumptions or proceed without user confirmation on implementation choices, preferences, or ambiguous requirements.

## Quick Commands

```bash
# Build and test
go build ./...
go test ./...
go test -race -count=1 ./...

# Run specific test
go test ./... -run TestNew
go test ./pkg/cas -run TestNew

# Coverage (85% minimum required)
./coverage.sh

# Heavy testing suite
./testHeavy.sh

# Lint and format
golangci-lint run
golangci-lint run --fix
go mod tidy
```

## Function Annotations (Mandatory)

All functions MUST have authorship annotations on the same line as `func`:

- `// A` - AI-authored, no review
- `// AP` - AI-authored, reviewed with TODO
- `// AC` - AI-authored, reviewed and approved
- `// H` - Human-authored
- `// HP` - Human-authored with TODO
- `// HC` - Human-comprehended and confident

Production-critical functions (prefix with `P`):
- `// PAP`, `// PAC`, `// PHC` - Must reach `// PHC` before production

Example:
```go
func exampleFunction() { // A
    // Low-risk, AI-authored
}

func criticalFunction() { // PHC
    // High-risk, production-ready
}
```

## Code Style

**Formatting:**
- Line length: 80 characters max
- Cyclomatic complexity: max 10
- Use `gofumpt`, `goimports`, `gci`, `golines`

**Imports (grouped):**
```go
import (
    // stdlib
    "context"
    
    // third-party
    "github.com/example/lib"
    
    // local
    "github.com/i5heu/ouroboros-db/pkg/interfaces"
)
```

**Naming:**
- Test functions: `TestXxx`, `BenchmarkXxx`, `ExampleXxx`
- Table-driven tests preferred
- Property tests with `pgregory.net/rapid`

## Logging Policy

**Always use `pkg/clusterlog`** for logging. Exception only when it would create subscription loops (document with `// LOGGER` comment).

**sloglint rules:**
- kv-only (no mixed args)
- context-aware methods
- Static messages
- camelCase keys
- lowercased messages
- No raw keys

## Testing Requirements

- Coverage: 85% minimum (files, packages, total)
- Tests live in `_test.go` files alongside code
- Use table-driven tests
- Property testing with `pgregory.net/rapid`
- Set `RAPID_CHECKS=10_000` for thorough property testing

## Project Structure

```
├── ouroboros.go       # Root type and constructor
├── config.go          # Config struct
├── pkg/               # Public packages
│   ├── cas/           # CAS primitives
│   ├── clusterlog/    # Logging (use this!)
│   └── interfaces/    # Interface definitions
├── internal/          # Internal services
├── cmd/               # Binaries
└── benchmark/         # Benchmarking tools
```

## Linting Configuration

Linters enabled in `.golangci.yml`:
- `govet`, `errcheck`, `staticcheck`, `unused`, `ineffassign`
- `gosec` - Security
- `sloglint` - Structured logging
- `cyclop` - Complexity (max 10)
- `lll` - Line length (80 chars, tab-width 2)

Formatters auto-applied with `--fix`:
- `gofumpt`, `goimports`, `gci`, `golines`, `gofmt`

## Key Dependencies

- `github.com/i5heu/ouroboros-crypt` - Encryption
- `github.com/dgraph-io/badger/v4` - KV store
- `github.com/klauspost/reedsolomon` - Erasure coding
- `pgregory.net/rapid` - Property testing

## PR Checklist

1. Add tests demonstrating behavior
2. Add function annotation (`// A`, `// AC`, etc.)
3. Run `go test ./...` and `golangci-lint run`
4. Add/update `Example...` tests for public API changes
5. Mark production-critical changes with `P` prefix and request human review

## Important Files

- `CLAUDE.md` - Full architecture overview
- `.github/copilot-instructions.md` - AI developer instructions
- `docs/diagrams/architecture.mmd` - Architecture diagram
- `.golangci.yml` - Linting rules
- `.github/.testcoverage.yml` - Coverage thresholds
