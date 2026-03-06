# Contributing

Thanks for contributing to `pipeline-mcp`.

## Ground Rules

- Use a feature branch and open a pull request against `main`.
- Keep changes small and focused. Large changes should be split into reviewable slices.
- Add or update tests for behavior changes.
- Update documentation when public behavior, configuration, or workflows change.
- Do not include secrets, credentials, or sensitive logs in commits, issues, or pull requests.

## Development Setup

1. Install Go `1.26.1` or newer.
2. Clone the repository.
3. Run `go test ./...`.
4. Run `go run ./cmd/benchmark` if you change diagnosis logic or benchmark fixtures.
5. Build the server with `go build -o bin/pipeline-mcp ./cmd/pipeline-mcp`.

## Pull Requests

Use the pull request template and include:

- The problem being solved.
- The smallest viable change set.
- Test coverage or benchmark updates.
- Any GitHub settings or operational follow-ups.

The `main` branch is protected. Pull requests must pass required checks before merge.

## Issues

Use the issue templates for bugs and feature requests. Security vulnerabilities should be reported privately according to [SECURITY.md](SECURITY.md).

## Benchmarks and Fixtures

Diagnosis changes should include a reproducible fixture under `testdata/benchmarks/historical_failures.json` when possible. That keeps regressions visible in CI.
