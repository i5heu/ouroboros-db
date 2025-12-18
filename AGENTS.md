# Repository Guidelines

## Project Structure & Module Organization
- Library root (`ouroboros.go`, `config.go`) exposes `OuroborosDB` with a simple `Config` for paths, logging, and UI port selection.
- Public packages live in `pkg/`: `pkg/cas` implements content-addressable storage primitives (blobs, chunks, sealed slices) and `pkg/logging` wraps structured logging.
- Internal services live in `internal/` (`api`, `cluster`, `dataRouting`, `health`, `index`, `node`, `rebalance`, `transport`, `testutil`) and may change without notice; keep new features out of `internal` unless stability is not required.
- Entry points in `cmd/cli` and `cmd/daemon` are placeholders for tooling; add commands there instead of the library root.
- Benchmarks sit in `benchmark/` (Docker-based runner plus HTML template) with generated results in `docs/bench-html/`. Heavy local runs are orchestrated by `testHeavy.sh`.

## Build, Test, and Development Commands
- Use Go 1.24.6. Standard check: `go build ./...` to ensure all packages compile.
- Fast tests: `go test ./...`; race/soak: `go test -race -count=1 ./...` or `./testHeavy.sh` for race, repeated runs, and benches.
- Benchmarks only: `go test -bench=. -benchtime=10s ./...` or Dockerized comparisons via `./benchmarkVersions.bash` (writes results to `docs/bench-html/`).
- Keep dependencies tidy with `go mod tidy` after adding imports. Format with `gofmt -w` or `go fmt ./...`.

## Coding Style & Naming Conventions
- Rely on Go defaults: tabs for indentation, CamelCase for exported identifiers, package names short and lower-case.
- Apply the annotation legend from `README.md`: append `// A`, `// AC`, `// H`, etc. to `func` declarations to record authorship/review state; upgrade high-risk paths to `PHC` before release.
- Tests live alongside code in `_test.go` files; favor table-driven patterns; prefer slog-based logging via `pkg/logging` or `defaultLogger`.

## Testing Guidelines
- Primary framework is Go’s `testing` package; `internal/testutil` hosts helpers.
- Coverage gates in `.github/.testcoverage.yml` set 50% minimum for files, packages, and total. Generate profiles with `go test -coverprofile=cover.out ./...`.
- Name tests `TestXxx`, benchmarks `BenchmarkXxx`, and example snippets `ExampleXxx` to keep tooling discoverable.

## Commit & Pull Request Guidelines
- Follow the existing log style: concise, imperative messages with optional Conventional Commit prefixes (e.g., `feat: ...`, `docs: ...`); keep scope small.
- PRs should describe motivation, summarize major code paths touched, link issues, and list how to reproduce or validate (tests, benches). Note any new configuration knobs or data-directory expectations.
- Before opening a PR, run formatting, `go test ./...`, and (when touching hot paths) the race suite; include function annotations for new or changed logic so reviewers can gauge review depth.

## Security & Configuration Tips
- Do not commit real data paths or secrets; `Config.Paths` should point to local scratch directories and only `Paths[0]` is used today.
- Logging defaults to stderr; set a custom `*slog.Logger` in `Config` for structured output and to control verbosity.
