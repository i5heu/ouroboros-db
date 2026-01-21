# AGENTS.md

This file provides guidelines for agentic coding assistants working in the OuroborosDB repository.

## Build/Lint/Test Commands

### Testing
- Run all tests: `go test ./...`
- Run single test: `go test ./pkg/<name> -run TestName`
- Run package tests: `go test ./internal/carrier`
- Run with race detector: `go test -race ./...`
- CI uses `go test ./...` as ground truth

### Building
- Build all packages: `go build ./...`
- Build specific binary: `go build ./cmd/daemon`

### Formatting & Linting
- Format code: `gofmt -w .`
- Vet code: `go vet ./...`
- Full lint: `golangci-lint run`
- Run formatters: `golangci-lint run --fix`

### Benchmarks
- Run benchmarks: `benchmark/run_benchmarks.py`
- Benchmark setup: see `benchmark/Dockerfile`

## Code Style Guidelines

### Function Annotations (REQUIRED)
All function declarations must include an authorship/review status annotation on the same line:
- `// A` - AI-authored, no human review
- `// AP` - AI-authored, human-reviewed with TODO
- `// AC` - AI-authored, human reviewed and approved
- `// H` - Human authored
- `// HP` - Human authored with TODO
- `// HC` - Human comprehended and confident

For high-risk functions (complex algorithms, security-sensitive, critical data handling), add `P` prefix:
- `// P[A|AP|AC|H|HP|HC]` - Priority/critical function
- All `P` functions must reach `PHC` status before production release

Example:
```go
func New(conf Config) (*OuroborosDB, error) { // A
    // Implementation
}

func criticalFunction() { // PHC
    // High-risk implementation
}
```

### Formatting & Line Length
- Max line length: 80 characters (strictly enforced by lll linter)
- Use 2-space indentation
- Use `gofumpt` with extra-rules enabled
- Use `goimports` for import management
- Use `gci` for import grouping

### Imports
- Group imports: stdlib, third-party, internal (via gci)
- Use fully qualified paths (no dot imports)
- Remove unused imports (checked by unused linter)

### Logging
- Use `log/slog` package
- Key-value pairs only (no attributes)
- No global loggers
- Context required for all log calls
- Static, lowercased messages
- CamelCase key names
- Example: `slog.InfoContext(ctx, "node connected", "nodeID", nodeID)`

### Types & Naming
- CamelCase for exported names
- camelCase for unexported
- Max cyclomatic complexity: 10
- Prefer explicit types over type inference when clarity is needed
- Interface names: simple noun (e.g., `Carrier`, `Transport`)

### Error Handling
- Always check errors (enforced by errcheck)
- Wrap errors with context: `fmt.Errorf("operation: %w", err)`
- Avoid ignoring errors with blank identifier

### Testing
- Write unit tests for logic changes
- Write integration tests for storage/encryption/sync changes
- Simulate concurrent heads for cluster tests
- Use table-driven tests for multiple test cases
- Test utilities in `internal/testutil/`

### Package Structure
- Library code: root `ouroboros` package
- Public utilities: `pkg/`
- Internal helpers: `internal/`
- Binaries: `cmd/`

### Code Quality
- Use govet for static analysis
- Use staticcheck for additional checks
- Use gosec for security scanning
- Prefer idempotent, retry-safe operations
- Keep functions simple and focused

### Project-Specific Notes
- Entrypoint: `ouroboros.New(conf Config)` in `ouroboros.go`
- Data path config: `Config.Paths[0]`
- Message types: `pkg/carrier/message.go`
- Storage flow: `pkg/storage/cas.go`
- Architecture diagrams: `docs/diagrams/Architecture.mmd`

### PR Requirements
- Add unit tests for logic changes
- Add integration tests for storage/encryption/sync changes
- P codepaths require PHC sign-off and explicit risk note in PR
- Update README.md and docs/diagrams/Architecture.mmd for architectural changes
