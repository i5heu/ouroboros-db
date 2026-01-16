<!--
Copilot instructions file for the OuroborosDB repo.
Keep short and concrete. Refer to project files and patterns discovered in the repo.
-->

# Copilot & AI Developer Instructions — OuroborosDB

Summary

- Go-based content-addressable DB: DAG of Blob → Chunk → SealedSlice. Root package: `ouroboros` (`ouroboros.go`, `config.go`).
- Core flows: content-defined chunking (`pkg/model/chunk.go`), encryption/KeyEntry (`pkg/encryption`), and block assembly + region-based access (`pkg/model/block.go`, `pkg/storage/blockstore.go`).
- Cluster model: mostly stateless nodes; use SyncIndexTree (`pkg/rebalance`) + DeletionWAL (`pkg/datarouter`) for convergence without global consensus.

Quick pointers (what to edit and where) 🔧

- Entrypoint constructor: `ouroboros.New(conf Config)` in `ouroboros.go`.
- Configuration: prefer `Config.Paths[0]` for data path; `UiPort` exposes the built-in UI (`config.go`).
- Storage flow example: see `pkg/storage/cas.go` (chunk → SealChunk → WAL buffer → block store).
- Communication: message types in `pkg/carrier/message.go` (ChunkMetaRequest, KeyEntryRequest) — follow these for transport changes.

Conventions & code hygiene ✅

- Authorship annotations are REQUIRED and inline on the func declaration: `func Foo() { // A|AP|AC|H|HP|HC }`. Use `P` prefix for priority functions and require `PHC` before release.
- Package layout: library code in the repo root `ouroboros`, public utilities under `pkg/`, internal helpers under `internal/`, and binaries under `cmd/`.
- Prefer idempotent, retry-safe operations (nodes are stateless-ish).

Developer workflows ⚙️

- Run unit tests: `go test ./...` (CI uses this). For per-package: `go test ./pkg/<name> -run TestName`.
- Build: `go build ./...`; format/lint: `gofmt`/`go vet` (follow CI failures as ground truth).
- Benchmarks: `benchmark/run_benchmarks.py` + `benchmark/Dockerfile` for performance tests.

Project conventions & critical rules

- Annotation scheme (core convention): all function comments must reflect authorship/review status in the function declaration, included in `README.md`:
  - `// A` AI-authored, no human review
  - `// AP` AI-authored, human-reviewed w/ TODO
  - `// AC` AI-authored, human reviewed and approved
  - `// H` Human authored, `// HP` human authored w/ TODO
  - `// HC` Human comprehended & confident
  - `P` prefix denotes **priority/critical** functions. e.g., `// PHC` means production-critical and human-reviewed.
- It is negotiable that AI generated functions must be generated with an `// A` or `// AP` annotation after the function declaration `func exampleFunction() { // A`.

PR guidance & quality gates 🚦

- Add unit tests for logic changes; for storage/encryption/sync changes add an integration test that simulates concurrent heads.
- Changes touching `P` codepaths require `PHC` sign-off and an explicit risk note in the PR.
- Update `README.md` and `docs/diagrams/Architecture.mmd` for any architectural changes.

Example: create a DB

```go
conf := ouroboros.Config{Paths: []string{"/tmp/ouroboros"}}
db, err := ouroboros.New(conf)
```

If something is unclear, open an issue or ask a maintainer — reference the function annotation (e.g., `// A` or `// AP`) for review prioritization.
