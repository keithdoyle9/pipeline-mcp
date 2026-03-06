# Operator Guide

`pipeline-mcp` is intended to run in a read-only mode by default. Enable mutations only when you have an explicit rerun use case and an audit destination you trust.

## GitHub Credentials

| Variable | Required | Minimum access | Used by |
| --- | --- | --- | --- |
| `GITHUB_READ_TOKEN` | Recommended | `actions:read` | `pipeline.get_run`, `pipeline.diagnose_failure`, `pipeline.analyze_flaky_tests`, `pipeline.compare_performance` |
| `GITHUB_WRITE_TOKEN` | Only when `DISABLE_MUTATIONS=false` | `actions:write` | `pipeline.rerun` |
| `GITHUB_TOKEN` / `GH_TOKEN` | Optional fallback | `actions:read` when used | Fallback for `GITHUB_READ_TOKEN` only |

Operational guidance:

- Prefer separate read and write tokens so read-only hosts do not carry write credentials.
- Keep `DISABLE_MUTATIONS=true` unless the host is explicitly allowed to trigger reruns.
- `GITHUB_WRITE_TOKEN` must be explicit whenever reruns are enabled; `GITHUB_TOKEN` and `GH_TOKEN` are read-token fallbacks only.
- Read tools may work against public repositories without a token, but rate limits and private-repository access will be worse.

## Runtime Modes

### Read-only default

- `DISABLE_MUTATIONS=true`
- Set `GITHUB_READ_TOKEN` with `actions:read`
- Omit `GITHUB_WRITE_TOKEN`

### Controlled rerun mode

- `DISABLE_MUTATIONS=false`
- Set `GITHUB_READ_TOKEN` with `actions:read`
- Set `GITHUB_WRITE_TOKEN` with `actions:write`
- Set `AUDIT_LOG_PATH` to a persistent location
- Set `AUDIT_SIGNING_KEY` when tamper-evident audit logs are required

If `DISABLE_MUTATIONS=false` without a write token, startup now fails fast so the server does not advertise a broken mutation path.

## Filesystem Expectations

- `AUDIT_LOG_PATH` should point to persistent storage on the host that is allowed to keep `0600` file permissions.
- `METRICS_EXPORT_PATH` is optional; set it only when you want JSON metric snapshots written to disk.
- Audit log directories are created with `0700` permissions, and the audit log file is forced to `0600`.
- When `AUDIT_SIGNING_KEY` is unset, audit entries are still written, but the `signature` field is omitted.

## Release Verification

Use the pinned release verification script before cutting a tag:

```bash
./scripts/verify-release.sh
```

The script runs `go vet`, tests, benchmark fixtures, `govulncheck`, single-binary compilation, `goreleaser check`, and `goreleaser release --snapshot --clean`. If `govulncheck` or `goreleaser` are not already installed, it falls back to pinned `go run` invocations.

## Runbook Ownership

Current owners for the MVP release:

- Provider outage triage: repository maintainer / release owner
- GitHub token rotation: repository maintainer / repository admin

Minimum incident response steps:

1. For GitHub API outages or rate-limit incidents, keep `DISABLE_MUTATIONS=true`, capture the error envelope, and monitor GitHub status before retrying.
2. For credential rotation, mint the replacement token with the same minimum scope, update the secret store, restart the server if needed, and validate read or rerun access with a known repository.
