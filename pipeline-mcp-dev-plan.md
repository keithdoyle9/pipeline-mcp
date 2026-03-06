# pipeline-mcp MVP Development Plan (1–2 Day Slices)

## Summary
- Build an MVP Go MCP server for GitHub Actions only, implementing diagnosis, flaky-test analysis, controlled reruns, and performance comparisons from the PRD.
- Execute in small vertical slices (1–2 days each), each ending in runnable behavior plus tests.
- Keep post-MVP work as a parking lot only (no implementation in this plan).
- Primary success gates: AC-1 through AC-5, latency targets (`<=2s` metadata p95, `<=8s` log-analysis p95 for 20 MB), and reliable structured errors.

## Public Interfaces (Locked for MVP)
- MCP tools:
1. `pipeline.get_run(run_url|run_id) -> PipelineRun`
2. `pipeline.diagnose_failure(run_url|run_id, max_log_bytes) -> FailureDiagnostic, FixRecommendation[]`
3. `pipeline.analyze_flaky_tests(repository, lookback_days, workflow?) -> FlakyTestReport`
4. `pipeline.rerun(run_id, failed_jobs_only, reason) -> RerunResult`
5. `pipeline.compare_performance(repository, workflow, from, to) -> PipelinePerformanceSnapshot`
- Shared error envelope: `code`, `message`, `remediation`, `retryable`, `details`.
- MVP error codes: `UNAUTHORIZED`, `LOG_UNAVAILABLE`, `RATE_LIMITED`, `INVALID_INPUT`, `PROVIDER_UNAVAILABLE`, `INTERNAL`.
- Audit event contract for mutations: `event_id`, `tool`, `actor`, `repository`, `run_id`, `reason`, `scope`, `timestamp`, `outcome`.

## Implementation Slices (Small Work Items)
1. **Slice 01: Repo and server skeleton** — Initialize Go module, MCP server bootstrap, config loader, basic logging, tool registration stubs; done when server starts and advertises all 5 tool names.
2. **Slice 02: Domain models and validation** — Implement core PRD types and input validation helpers; done when invalid/missing input paths produce standardized `INVALID_INPUT`.
3. **Slice 03: GitHub auth + API client** — Add read/write token separation, client wrapper, retry/backoff, rate-limit handling; done when read endpoints work and write calls are blocked without write token.
4. **Slice 04: `pipeline.get_run`** — Parse run URL or run ID, fetch run metadata/jobs, normalize to `PipelineRun`; done when run lookup succeeds and unauthorized/rate-limited cases return structured errors.
5. **Slice 05: Log ingestion + redaction** — Implement bounded log fetch (`max_log_bytes`), streaming parse, deterministic secret redaction; done when large logs are truncated safely and redaction tests pass.
6. **Slice 06: Diagnosis engine v1 + recommendations** — Add failure categorization heuristics, evidence extraction, confidence scoring, recommendation mapping; done when `pipeline.diagnose_failure` returns ranked root cause and evidence links.
7. **Slice 07: Flaky test analyzer** — Aggregate historical failures with lookback/workflow filters, compute frequency/recency/confidence; done when `pipeline.analyze_flaky_tests` returns top flaky tests with confidence.
8. **Slice 08: Controlled rerun + audit trail** — Implement `pipeline.rerun` with explicit scope (`failed_jobs_only` or full), required `reason`, persistent audit event; done when reruns are guarded and auditable.
9. **Slice 09: Performance comparison tool** — Implement baseline/current metric computation (queue time, duration, success rate, failure categories, flaky rate); done when `pipeline.compare_performance` returns both windows and deltas.
10. **Slice 10: Telemetry and observability** — Add per-tool latency/error/confidence metrics and structured audit logging; done when dashboards/exports expose p95 latency and error rate by tool.
11. **Slice 11: Benchmark harness + acceptance suite** — Add historical-failure benchmark runner for top-3 diagnosis accuracy and latency/load scripts; done when benchmark outputs are reproducible in CI.
12. **Slice 12: Hardening and release prep** — Final security checks, docs for auth scopes/operations, packaging as single binary; done when MVP release checklist is green.

## Test Plan
1. Contract tests for all tool inputs/outputs and error envelopes.
2. Unit tests for URL parsing, normalization, redaction, classification, scoring, and flaky aggregation logic.
3. Integration tests against mocked GitHub endpoints for metadata, logs, reruns, and rate-limit/permission paths.
4. Performance tests for metadata and log-analysis latency targets at realistic concurrency and 20 MB logs.
5. Acceptance tests mapped directly to AC-1 through AC-5.
6. Regression benchmark run for diagnosis top-3 accuracy and failure-class precision/recall.

## Assumptions and Defaults
- Current repo starts from near-empty state; all implementation scaffolding is included in this plan.
- MVP diagnosis is deterministic/rule-based (no external LLM dependency) to control latency and reproducibility.
- GitHub is the only provider in scope; GitLab/CircleCI/Jenkins/ArgoCD are parked.
- Mutation tools are disabled by default unless write-scoped credentials are explicitly configured.
- Parking lot (not scheduled in MVP slices): GitLab/CircleCI adapters, richer taxonomy/recommendation policy controls, deployment intelligence.
