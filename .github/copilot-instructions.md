# Copilot & AI Developer Instructions — OuroborosDB 🔧

## Communication Guidelines

**Always use the `question` tool** to ask users for clarification or decisions before taking actions. Do not make assumptions or proceed without user confirmation on implementation choices, preferences, or ambiguous requirements.

Short summary
- OuroborosDB is a Go content-addressable DB using a Git-like DAG: Blob → Chunk → SealedSlice. The root library package is `ouroboros`; stable helpers live in `pkg/`; changeable internals live in `internal/`; CLIs/daemons go under `cmd/`.

Quick commands (local parity with CI) ✅
- Run tests: `go test ./...`
- Race detector: `go test -race -count=1 ./...`
- Coverage: `./coverage.sh`
- Heavy suite (maintainers): `./testHeavy.sh`
- Format & lint: `gofumpt`/`goimports`; `golangci-lint run` (`--fix` allowed for auto-fixes)

Architecture essentials (what to read first) 📚
- Read `README.md` and `docs/diagrams/architecture.mmd` for the big picture.
- Key patterns: content-addressed objects, SyncIndexTree, DeletionWAL, DataRouter / LocalKVStore, and idempotent, retry-safe operations.
- Important files: `ouroboros.go`, `config.go` (note `Config.Paths[0]` usage), `pkg/clusterlog` (logging), `AGENTS.md` (detailed dev rules).

Repo conventions you must follow ⚠️
- Function authorship/review annotation is mandatory on the same `func` line. Examples:
  - `func New(conf Config) (*OuroborosDB, error) { // A`  (AI authored)
  - `func criticalOp() { // PHC`  (production-critical, human-reviewed)
- Logging: prefer `pkg/clusterlog` and follow `sloglint` rules (kv-only, context-aware, static messages, camelCase keys). See `pkg/clusterlog/cluster_log_test.go` for examples and `interfaces.Carrier` usage.
- Tests: add unit tests, use `Example...` functions for small docs (`pkg/clusterlog/example_test.go`), and use property tests with `pgregory.net/rapid` (if local fork is needed, add the replace in `go.mod` as in tests).

Testing patterns & deliberate choices 🧪
- Use table-driven tests, property tests (rapid), and add integration tests when state/replication semantics are involved.
- Example rapid fork snippet you may need in `go.mod`:
  require pgregory.net/rapid v1.2.0
  replace pgregory.net/rapid => github.com/flyingmutant/rapid v1.2.0

PR checklist for AI authors ✅
1. Add tests that demonstrate the behavior (unit, property, integration as needed).
2. Add function annotation (`// A` / `// AP` / etc.) on the `func` line.
3. Run `go test ./...` and `golangci-lint run`; fix issues (follow `AGENTS.md` lint list).
4. Add or update `Example...` tests and top-level docs (`README.md`/`AGENTS.md`) for public API changes.
5. Mark production-critical changes with `P` annotations and request human review to reach `PHC`.

Where to put code & integration points 🔗
- Library/core changes → `ouroboros` package
- New public packages → `pkg/`
- Implementation details → `internal/`
- Binaries → `cmd/<name>`
- Inspect `pkg/clusterlog` and `interfaces` for logging and carrier patterns; reuse `mockCarrier` in tests.

If unsure or blocked
- Open an issue or ping a maintainer before changing critical flows (replication, encryption, deletion WAL). For architecture questions, read `CLAUDE.md` and `AGENTS.md` first.

Short example — create DB
```go
conf := ouroboros.Config{Paths: []string{"/tmp/ouroboros"}}
db, err := ouroboros.New(conf) // A
```

Feedback? Tell me what’s unclear or missing and I’ll iterate. ✨
