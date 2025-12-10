<!--
Copilot instructions file for the OuroborosDB repo.
Keep short and concrete. Refer to project files and patterns discovered in the repo.
-->
# Copilot & AI Developer Instructions — OuroborosDB

Summary
- OuroborosDB is a small, Go-based content-addressable database library (DAG of blobs → chunks → sealed slices).
- The single root package is `ouroboros` (see `ouroboros.go`, `config.go`, `cluster.go`), and CLI/daemon binaries live under `cmd/`.
- The design centers on a Git-like DAG, index synchronization (SyncIndexTree), and a deletion WAL. See `README.md` for diagrams and more.

Key files + architecture pointers
- `ouroboros.go` contains the root type `OuroborosDB` and constructor `New(conf Config)`.
- `config.go` contains `Config` and `defaultLogger()` — `Config.Paths[0]` is the only path used today; `UiPort` controls the built-in UI.
- `cluster.go` (top-level clustering concerns) + `cmd/` (cli/daemon) show planned entrypoints; `internal/` and `pkg/` are for library and internal code.
- `go.mod` references an external crypto module: `github.com/i5heu/ouroboros-crypt` — check this for crypto patterns.

Project conventions & critical rules
- Annotation scheme (core convention): all function comments must reflect authorship/review status in the function declaration, included in `README.md`:
  - `// A` AI-authored, no human review
  - `// AP` AI-authored, human-reviewed w/ TODO
  - `// AC` AI-authored, human reviewed and approved
  - `// H` Human authored, `// HP` human authored w/ TODO
  - `// HC` Human comprehended & confident
  - `P` prefix denotes **priority/critical** functions. e.g., `// PHC` means production-critical and human-reviewed.
- All AI-generated functions must be marked `// A` or `// AP`. Do not remove or change this unless explicitly requested by a reviewer.

Patterns relevant to AI coding agents
- Use the `ouroboros` package root for library changes (not random top-level packages). If adding binaries, put code under `cmd/<binary>`.
- Persisted data model: Blob → Chunk → SealedSlice. Local KV stores map hashes to stored objects — check this model when designing storage logic.
- Indexing & sync: Distributed `SyncIndexTree` and `DeletionWAL` exist to make nodes converge — use their interfaces for replication changes.
- Nodes are intended to be mostly stateless; prefer designing idempotent operations for replication and replays.

Testing & CI
- GitHub Actions: `.github/workflows/go.yml` runs `go test ./...` and coverage. Use the same coverage and test steps locally for parity.
- Add unit tests when adding logic; `go test ./...` is the canonical command.

Integration & external deps
- Crypto: `github.com/i5heu/ouroboros-crypt` is used for sealing/encryption — inspect that module for expected APIs and types.
- Indirect deps like `cloudflare/circl` may be used by the crypto module; do not edit those direct/in-direct deps lightly.

Architecture/Design hints for AI PRs
- Start by reading `README.md` architecture section; changes should respect the DAG + index approch and the DataRouter / LocalKVStore pattern.
- If adding storage or network code, preserve idempotency and make operations safe to retry (stateless-ish nodes).
- When designing APIs that modify persistent state, include unit tests and, if relevant, an integration test that simulates concurrent writes creating separate heads.

Reporting & merging
- For production/priority code paths (marked with `P`), require `PHC` status before a release — add tests and manual review.
- Document new public APIs in top-level docs or README if they change primary behavior.

If anything is unclear
- Ask a maintainer if unsure where to put code (pkg vs internal vs cmd) or when adding critical operations.
- Check `README.md` for the full annotation legend and architecture diagram. If you’d like more rules codified here (linting, PR labels, or GH checks) say so and we’ll add them.

Short example: create a new DB
```go
conf := ouroboros.Config{Paths: []string{"/tmp/ouroboros"}}
db, err := ouroboros.New(conf)
if err != nil { // handle err }
```

Thanks — leave feedback or request additional rules (linters, commit message format, more test harnesses) and I’ll update this guidance.
