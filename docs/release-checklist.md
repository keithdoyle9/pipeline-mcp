# MVP Release Checklist

Use [docs/operator-guide.md](docs/operator-guide.md) for auth scope and operational guidance, and run `./scripts/verify-release.sh` for the local release gate.

## Security

- [ ] `DISABLE_MUTATIONS` defaults to `true` in production config.
- [ ] `GITHUB_READ_TOKEN` scope is limited to `actions:read`.
- [ ] `GITHUB_WRITE_TOKEN` is only configured where rerun is allowed (`actions:write`).
- [ ] `GITLAB_READ_TOKEN` scope is limited to `read_api` where GitLab access is required.
- [ ] `GITLAB_WRITE_TOKEN` is only configured where GitLab failed-job retries are allowed (`api`).
- [ ] Audit log path is persistent and protected.
- [ ] `AUDIT_SIGNING_KEY` is configured anywhere tamper-evident audit logs are required.
- [ ] Log redaction tests pass.

## Reliability

- [ ] `go test ./...` passes.
- [ ] `./scripts/run-benchmarks.sh` runs successfully.
- [ ] Metrics export path is configured and writable if telemetry export is required.
- [ ] Retry/backoff behavior is validated against GitHub and GitLab rate-limit and transient failures.

## Operations

- [ ] Run `./scripts/verify-release.sh` to execute the local release gate.
- [ ] Build single binary: `go build -o bin/pipeline-mcp ./cmd/pipeline-mcp`.
- [ ] Validate release config: `goreleaser check`.
- [ ] Validate snapshot archives and checksums: `goreleaser release --snapshot --clean`.
- [ ] Confirm GitHub Release contains `darwin` and `linux` archives for `amd64` and `arm64`, plus `checksums.txt`.
- [ ] Confirm prerelease tags such as `v0.1.0-rc.1` are published as GitHub prereleases.
- [ ] Verify server startup and MCP tool listing with target client.
- [ ] Confirm audit event format includes actor, reason, scope, timestamp, and outcome.
- [ ] Confirm GitLab reruns are limited to `failed_jobs_only=true`.
- [ ] Confirm runbook owners for provider outage and credential rotation in [docs/operator-guide.md](docs/operator-guide.md).

## Green Criteria

Treat the MVP release checklist as green only when all of the following are true:

- The local release gate (`./scripts/verify-release.sh`) or the equivalent CI packaging job passes on the release commit.
- The tag-driven GitHub release workflow publishes the expected archives plus `checksums.txt`.
- Operator sign-off confirms token scopes, mutation posture, and runbook ownership in [docs/operator-guide.md](docs/operator-guide.md).
