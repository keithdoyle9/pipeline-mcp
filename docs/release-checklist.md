# MVP Release Checklist

## Security

- [ ] `DISABLE_MUTATIONS` defaults to `true` in production config.
- [ ] `GITHUB_READ_TOKEN` scope is limited to `actions:read`.
- [ ] `GITHUB_WRITE_TOKEN` is only configured where rerun is allowed (`actions:write`).
- [ ] Audit log path is persistent and protected.
- [ ] Log redaction tests pass.

## Reliability

- [ ] `go test ./...` passes.
- [ ] `./scripts/run-benchmarks.sh` runs successfully.
- [ ] Metrics export path is configured and writable if telemetry export is required.
- [ ] Retry/backoff behavior is validated against GitHub rate-limit and transient failures.

## Operations

- [ ] Build single binary: `go build -o bin/pipeline-mcp ./cmd/pipeline-mcp`.
- [ ] Verify server startup and MCP tool listing with target client.
- [ ] Confirm audit event format includes actor, reason, scope, timestamp, and outcome.
- [ ] Confirm runbook owners for provider outage and credential rotation.
