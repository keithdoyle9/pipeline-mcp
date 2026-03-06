# pipeline-mcp

[![CI](https://github.com/keithdoyle9/pipeline-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/keithdoyle9/pipeline-mcp/actions/workflows/ci.yml)
[![CodeQL](https://github.com/keithdoyle9/pipeline-mcp/actions/workflows/codeql.yml/badge.svg)](https://github.com/keithdoyle9/pipeline-mcp/actions/workflows/codeql.yml)
[![License](https://img.shields.io/github/license/keithdoyle9/pipeline-mcp)](LICENSE)

`pipeline-mcp` is a Go MCP server for CI/CD diagnosis and remediation workflows, starting with GitHub Actions.

## MVP Capabilities

- `pipeline.get_run`: normalize run metadata from `run_url`, `run_id + repository`, or `repository` alone for the latest failed run.
- `pipeline.diagnose_failure`: classify failure category and return ranked fix recommendations with evidence from `run_url`, `run_id + repository`, or `repository` alone for the latest failed run.
- `pipeline.analyze_flaky_tests`: identify top flaky tests by frequency, recency, and confidence.
- `pipeline.rerun`: trigger controlled reruns with explicit reason and audit logging.
- `pipeline.compare_performance`: compare current window metrics against an immediately preceding baseline window.

## Open Source Defaults

- MIT licensed for reuse and contribution.
- Protected `main` branch with required pull requests and status checks.
- GitHub-owned Actions only, plus dependency review and CodeQL scanning.
- Dependabot updates for Go modules and GitHub Actions.
- Contributor workflow documentation in [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md), and [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).
- CODEOWNERS, issue templates, and a pull request template for consistent intake and review.

## Architecture

- `cmd/pipeline-mcp`: server entrypoint.
- `internal/githubapi`: GitHub Actions adapter (run metadata, jobs, logs, reruns, retries/backoff).
- `internal/analysis`: redaction, diagnosis heuristics, flaky analysis, performance aggregation.
- `internal/service`: orchestration, validation, tool error mapping.
- `internal/audit`: persistent JSONL audit events for mutation tools.
- `internal/telemetry`: in-memory latency/error/confidence tracking with optional export.
- `tools`: MCP tool contracts and handlers.

## Prerequisites

- GitHub token with `actions:read` for read tools
- Optional write token with `actions:write` for `pipeline.rerun`
- Go `1.26+` only if you are building from source

## Configuration

Environment variables:

- `SERVER_NAME` (default: `pipeline-mcp`)
- `VERSION` (default: release tag for official binaries, or `v0.1.0` for local source builds)
- `LOG_LEVEL` (`debug|info|warn|error`, default: `info`)
- `GITHUB_API_BASE_URL` (default: `https://api.github.com`)
- `GITHUB_READ_TOKEN` (recommended)
- `GITHUB_WRITE_TOKEN` (required for reruns)
- `GITHUB_TOKEN` or `GH_TOKEN` can be used as fallback for both read and write tokens
- `DISABLE_MUTATIONS` (default: `true`)
- `AUDIT_LOG_PATH` (default: `var/audit-events.jsonl`)
- `AUDIT_SIGNING_KEY` (optional HMAC key for tamper-evident audit signatures)
- `METRICS_EXPORT_PATH` (optional JSON snapshot path)
- `MAX_LOG_BYTES` (default: `20971520`)
- `DEFAULT_LOOKBACK_DAYS` (default: `14`)
- `MAX_HISTORICAL_RUNS` (default: `100`)
- `HTTP_TIMEOUT_SECONDS` (default: `25`)
- `USER_AGENT` (default: `pipeline-mcp/<version>`)
- `ACTOR` (default: `pipeline-mcp`)

When `AUDIT_SIGNING_KEY` is unset, audit entries omit the `signature` field rather than emitting a misleading unhashed digest.

## Install

Official release archives are published for `darwin` and `linux` on `amd64` and `arm64`.

Replace `VERSION` with the release tag you want to install.

```bash
VERSION=v0.2.0
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *)
    echo "unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

ARCHIVE="pipeline-mcp_${VERSION}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/keithdoyle9/pipeline-mcp/releases/download/${VERSION}"

curl -LO "${BASE_URL}/${ARCHIVE}"
curl -LO "${BASE_URL}/checksums.txt"
tar -xzf "${ARCHIVE}"
install -m 0755 pipeline-mcp /usr/local/bin/pipeline-mcp
```

Compare the SHA-256 for `pipeline-mcp_${VERSION}_${OS}_${ARCH}.tar.gz` with the matching line in `checksums.txt` before installing.

The server runs on stdio transport by default.

For Claude Code, attach the token when registering the MCP server:

```bash
claude mcp add --scope local -e GITHUB_READ_TOKEN=your_token_here -- pipeline-mcp "$(command -v pipeline-mcp)"
```

If you prefer to install from source with Go:

```bash
go install github.com/keithdoyle9/pipeline-mcp/cmd/pipeline-mcp@latest
```

## Build from Source

```bash
go build -o bin/pipeline-mcp ./cmd/pipeline-mcp
./bin/pipeline-mcp
```

Repository-only shortcut examples:

```text
Use pipeline.get_run with repository="owner/repo" to inspect the latest failed run.
Use pipeline.diagnose_failure with repository="owner/repo" to diagnose the latest failed run.
```

## GitHub Repository Protections

Recommended GitHub posture for this repository:

- Require pull requests for `main`.
- Require the `verify` and `dependency-review` checks to pass before merge.
- Disallow force pushes and branch deletion on `main`.
- Restrict GitHub Actions to GitHub-owned actions.
- Enable CodeQL, Dependabot security updates, secret scanning, and private vulnerability reporting.
- Keep workflow token permissions at `read` by default.

## Testing

```bash
go test ./...
```

Run benchmark harness:

```bash
./scripts/run-benchmarks.sh
```

Validate the release configuration locally:

```bash
goreleaser check
goreleaser release --snapshot --clean
```

Benchmark fixtures are in `testdata/benchmarks/historical_failures.json`.

## Tool Error Envelope

All tools return a structured error object when applicable:

- `code`
- `message`
- `remediation`
- `retryable`
- `details`

Supported codes for MVP:

- `UNAUTHORIZED`
- `LOG_UNAVAILABLE`
- `RATE_LIMITED`
- `INVALID_INPUT`
- `PROVIDER_UNAVAILABLE`
- `INTERNAL`

## Example Diagnosis

Sanitized example output from `pipeline.get_run` and `pipeline.diagnose_failure`:

```markdown
# Pipeline Failure Diagnosis

**Workflow**: Release Validation
**Run ID**: <run_id>
**Run URL**: https://github.com/<owner>/<repo>/actions/runs/<run_id>
**Commit**: `<commit_sha>`
**Diagnosed At**: <YYYY-MM-DD>
**Confidence**: 94%

## Root Cause

**Failure Category**: `config_error`
**Impacted Job**: `ios-release-candidate`

Branch `main` is blocked by environment protection rules for `<environment>`. Allowed deployment branches currently match: `release/*`.

## Evidence

1. `"Branch 'main' is not allowed to deploy to <environment> due to environment protection rules."`
2. `"The deployment was rejected or didn't satisfy other protection rules."`

## Fix Recommendations

1. Allow branch `main` to deploy to `<environment>`, or broaden the environment branch policy.
2. Run the workflow from a branch that matches `release/*`.
3. Route `main` dispatches to a less restricted environment instead of `<environment>`.
```

## Security Notes

- Log redaction is applied before diagnosis output.
- Mutation tools are disabled by default (`DISABLE_MUTATIONS=true`).
- Every rerun emits an auditable event with actor, reason, scope, and timestamp.

## Governance

- Contribution guide: [CONTRIBUTING.md](CONTRIBUTING.md)
- Security policy: [SECURITY.md](SECURITY.md)
- Code of conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- License: [LICENSE](LICENSE)
