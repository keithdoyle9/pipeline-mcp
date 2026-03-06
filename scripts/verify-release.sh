#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GORELEASER_VERSION="${GORELEASER_VERSION:-v2.14.1}"
GOVULNCHECK_VERSION="${GOVULNCHECK_VERSION:-v1.1.4}"

run_goreleaser() {
  # Prefer an installed GoReleaser for speed. The pinned go run fallback is the
  # reproducible path when the binary is not already present on the machine.
  if command -v goreleaser >/dev/null 2>&1; then
    goreleaser "$@"
    return
  fi

  GOFLAGS="${GOFLAGS:-}" go run "github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}" "$@"
}

run_govulncheck() {
  # Match the local fast-path/fallback behavior used for GoReleaser above.
  if command -v govulncheck >/dev/null 2>&1; then
    govulncheck "$@"
    return
  fi

  GOFLAGS="${GOFLAGS:-}" go run "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}" "$@"
}

go vet ./...
go test ./... -count=1
./scripts/run-benchmarks.sh
run_govulncheck ./...
go build -o bin/pipeline-mcp ./cmd/pipeline-mcp
run_goreleaser check
run_goreleaser release --snapshot --clean
