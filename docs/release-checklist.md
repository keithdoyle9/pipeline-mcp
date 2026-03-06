# MVP Release Checklist

## Security

- [ ] `DISABLE_MUTATIONS` defaults to `true` in production config.
- [ ] `GITHUB_READ_TOKEN` scope is limited to `actions:read`.
- [ ] `GITHUB_WRITE_TOKEN` is only configured where rerun is allowed (`actions:write`).
- [ ] Audit log path is persistent and protected.
- [ ] `AUDIT_SIGNING_KEY` is configured anywhere tamper-evident audit logs are required.
- [ ] Log redaction tests pass.

## Reliability

- [ ] `go test ./...` passes.
- [ ] `./scripts/run-benchmarks.sh` runs successfully.
- [ ] Metrics export path is configured and writable if telemetry export is required.
- [ ] Retry/backoff behavior is validated against GitHub rate-limit and transient failures.

## Operations

- [ ] Build single binary: `go build -o bin/pipeline-mcp ./cmd/pipeline-mcp`.
- [ ] Validate release config: `goreleaser check`.
- [ ] Validate snapshot archives and checksums: `goreleaser release --snapshot --clean`.
- [ ] Confirm GitHub Release contains `darwin` and `linux` archives for `amd64` and `arm64`, plus `checksums.txt`.
- [ ] Confirm prerelease tags such as `v0.1.0-rc.1` are published as GitHub prereleases.
- [ ] Verify server startup and MCP tool listing with target client.
- [ ] Confirm audit event format includes actor, reason, scope, timestamp, and outcome.
- [ ] Confirm runbook owners for provider outage and credential rotation.
