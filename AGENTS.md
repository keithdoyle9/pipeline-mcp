# Repository Guidelines

## Project Structure & Module Organization
- `cmd/pipeline-mcp/`: server entrypoint (keep `main.go` and wiring only).
- `config/`: environment-driven runtime configuration (`config.go`).
- `internal/domain/`: shared contracts and response models.
- `internal/githubapi/`: GitHub Actions client, API types, and error mapping.
- `internal/analysis/`: deterministic analysis utilities (for example, log secret redaction).
- `testdata/benchmarks/`: fixtures for diagnosis/flaky/performance benchmark scenarios.
- `.github/workflows/`: CI, security scanning, and tagged release automation.
- `.goreleaser.yml`: release artifact definitions for official binaries.
- `scripts/` and `tools/`: local automation and helper utilities.
- `pipeline-mcp-dev-plan.md` and `pipeline-mcp-prd.md`: local planning notes only; keep them untracked and out of public commits.

## Build, Test, and Development Commands
- `go mod tidy`: sync module dependencies after import changes.
- `GOCACHE=$(pwd)/.gocache go build ./...`: compile all packages.
- `GOCACHE=$(pwd)/.gocache go test ./...`: run all tests.
- `GOCACHE=$(pwd)/.gocache go test ./... -cover`: run tests with coverage summary.
- `goreleaser check`: validate release configuration.
- `goreleaser release --snapshot --clean`: build local release archives and `checksums.txt` without publishing.
- `go run ./cmd/pipeline-mcp`: run the server locally once `main.go` is present.

## Coding Style & Naming Conventions
- Target Go `1.26.x` (see `go.mod`), and format with `go fmt ./...` before opening a PR.
- Keep package boundaries aligned with current layers (`domain`, `analysis`, `githubapi`); avoid cross-layer coupling.
- Naming: exported symbols use `PascalCase`; internal helpers use `camelCase`.
- Maintain stable JSON contracts with `snake_case` tags and the shared error envelope: `code`, `message`, `remediation`, `retryable`, `details`.
- Wrap errors with `%w`; keep common sentinel/API errors centralized in `internal/githubapi/errors.go`.

## Testing Guidelines
- Use Go’s standard `testing` package with table-driven tests for parsers, redaction patterns, and API error handling.
- Place tests next to source files with `*_test.go` naming.
- Keep reusable fixtures in `testdata/`; benchmark-style fixtures go in `testdata/benchmarks/`.
- Add regression tests for every bug fix, especially around URL parsing and retry/rate-limit branches.

## Commit & Pull Request Guidelines
- Follow concise, imperative commit subjects (current history example: `Add initial pipeline-mcp PRD`).
- Keep subject lines around 72 characters or fewer; include rationale in the body when behavior changes.
- PRs should include: short summary, linked issue/plan slice, test command output, and config impact (`GITHUB_READ_TOKEN`, `GITHUB_WRITE_TOKEN`, `DISABLE_MUTATIONS`).
- For contract/tool changes, include sample request/response payloads in the PR description.
