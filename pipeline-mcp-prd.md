# PRD: pipeline-mcp

## 1. Executive Summary

### Problem Statement
Teams increasingly use AI coding agents like Claude and Codex to generate code quickly, but when CI fails they still rely on slow, manual debugging across scattered pipeline logs and provider UIs. There is no dedicated MCP server focused on CI/CD diagnosis, remediation guidance, and pipeline performance analysis for AI-assisted development workflows.

### Proposed Solution
Build `pipeline-mcp`, a Go-based MCP server that gives Claude and Codex structured access to CI/CD pipeline data and actions, starting with GitHub Actions in MVP. The server enables natural-language debugging from a failed build URL, flaky test analysis, controlled reruns, and trend comparisons through well-defined MCP tool contracts.

### Success Criteria
- `<= 180s` median time from build URL input to first structured root-cause diagnosis for failed GitHub Actions runs.
- `>= 85%` top-3 diagnosis accuracy on a maintained benchmark set of historical pipeline failures.
- `>= 30%` reduction in repeat flaky-test failures in repositories actively using recommended fixes over a 90-day observation window.
- `>= 60%` weekly active usage among pilot repositories that use Claude/Codex for AI-assisted coding.
- `<= 2s` p95 response latency for metadata tools and `<= 8s` p95 latency for log-analysis tools on logs up to 20 MB.

## 2. User Experience & Functionality

### User Personas
- **AI-Assisted Application Developer (Claude/Codex user):** Writes code with AI agents and needs immediate, trustworthy CI failure diagnosis without manual log spelunking.
- **CI Platform Engineer:** Owns pipeline reliability and wants repeatable flaky test detection and performance trend visibility.
- **On-Call SRE/Release Engineer:** Needs safe, auditable rerun and deployment actions under incident pressure.

### User Stories
- **US-1:** As a developer using Claude or Codex, I want to paste a failed workflow URL and receive a probable root cause with prioritized fixes so I can unblock my PR quickly.
- **US-2:** As a CI platform engineer, I want flaky test analysis across recent pipeline history so I can reduce nondeterministic build failures.
- **US-3:** As an on-call engineer, I want to trigger controlled reruns from the agent with guardrails so I can restore pipeline health faster.
- **US-4:** As an engineering lead, I want pipeline performance trend comparisons so I can spot regressions after process or tooling changes.

### Acceptance Criteria
- **AC-1 (US-1):** Given a valid GitHub Actions run URL, the server returns `FailureDiagnostic` and `FixRecommendation[]` with confidence scores and evidence links in one tool call.
- **AC-2 (US-1):** If logs are unavailable or permissions are insufficient, the tool returns a structured error with actionable remediation (`UNAUTHORIZED`, `LOG_UNAVAILABLE`, `RATE_LIMITED`).
- **AC-3 (US-2):** Flaky test analysis returns top flaky tests, failure frequency, recency, and confidence using configurable lookback windows.
- **AC-4 (US-3):** Rerun operations require explicit scope (`failed_jobs_only` or `full_run`) and emit an audit event with actor, reason, and timestamp.
- **AC-5 (US-4):** Trend comparisons return baseline vs. current values for queue time, run duration, success rate, and failure categories.

### Non-Goals
- Building a full replacement UI for CI/CD providers.
- Auto-committing or auto-merging code changes to fix pipeline failures.
- Supporting GitLab CI, CircleCI, Jenkins, and ArgoCD in MVP (phased expansion only).
- Storing full repository source code for analysis.

## 3. AI System Requirements (If Applicable)

### Tool Requirements
- **Runtime:** Go-based MCP server for single-binary distribution and low resource overhead.
- **MVP Provider Integration:** GitHub Actions APIs for workflow runs, job logs, artifacts, reruns, and status metadata.
- **Core MCP Tool Contracts:**
  - `pipeline.get_run(run_url|run_id)` -> `PipelineRun`
  - `pipeline.diagnose_failure(run_url|run_id, max_log_bytes)` -> `FailureDiagnostic`, `FixRecommendation[]`
  - `pipeline.analyze_flaky_tests(repository, lookback_days, workflow?)` -> `FlakyTestReport`
  - `pipeline.rerun(run_id, failed_jobs_only, reason)` -> `RerunResult`
  - `pipeline.compare_performance(repository, workflow, from, to)` -> `PipelinePerformanceSnapshot`
- **Core Domain Types:**
  - `PipelineRun`: provider, repository, workflow, status, started_at, completed_at, duration_ms, run_url, commit_sha.
  - `FailureDiagnostic`: failure_category, suspected_root_cause, confidence, evidence_refs, impacted_jobs.
  - `FixRecommendation`: recommendation_id, description, expected_impact, confidence, references.
  - `PipelinePerformanceSnapshot`: success_rate, median_duration_ms, queue_time_ms, failure_breakdown, flaky_test_rate.
- **Agent Workflow Fit:** Outputs must be structured for Claude/Codex to consume directly for follow-up actions (rerun, recommendation selection, trend query).

### Evaluation Strategy
- Maintain a labeled benchmark corpus of historical GitHub Actions failures with known root causes.
- Track diagnosis precision/recall and top-3 accuracy by failure class (test failure, dependency outage, infra timeout, config error).
- Measure recommendation usefulness via acceptance rate and subsequent failure recurrence.
- Run latency/load benchmarks at varying log sizes and concurrent requests.
- Execute monthly regression tests to detect model/prompt drift in classification quality.

## 4. Technical Specifications

### Architecture Overview
- **Ingress Layer:** MCP transport and request validation.
- **Provider Adapter Layer (MVP: GitHub Actions):** Fetches run metadata, logs, artifacts, and mutation endpoints (rerun).
- **Normalization Layer:** Converts provider-native events/logs into common internal schemas.
- **Analysis Layer:** Classifies failure causes, detects flaky patterns, and generates ranked recommendations.
- **Telemetry & Audit Layer:** Captures tool usage, latency, confidence distributions, and action audit events.
- **Response Layer:** Returns typed MCP responses designed for Claude/Codex tool chaining.

Data flow:
1. Agent submits build URL or run ID.
2. Adapter fetches metadata/logs and normalizes records.
3. Analysis layer produces diagnosis and recommendations.
4. Optional mutation tools (rerun) execute with permission checks.
5. Response and audit events are emitted.

### Integration Points
- **CI Provider API (MVP):** GitHub Actions REST/GraphQL endpoints for runs, jobs, logs, reruns.
- **Authentication:** OAuth or GitHub App tokens with least-privilege scopes; separate read vs write permissions.
- **MCP Client Integrations:** Claude and Codex as primary target agents.
- **Observability:** Structured logs, metrics export (latency, error rate, diagnosis confidence), and audit trail storage.

### Security & Privacy
- Enforce least privilege for all provider credentials; write scopes are optional and disabled by default.
- Redact secrets/tokens/PII from logs before storage or transmission.
- Store only metadata and bounded log excerpts needed for diagnosis; configurable retention policies.
- Sign and persist audit events for every rerun/mutation action.
- Provide tenant/repository isolation in cache and telemetry storage.

## 5. Risks & Roadmap

### Phased Rollout
- **MVP:** GitHub Actions-only support; failure diagnosis from run URL, log retrieval, flaky test analysis, controlled rerun, and trend comparison APIs.
- **v1.1:** Add GitLab CI and CircleCI adapters, richer failure taxonomies, and configurable recommendation policies.
- **v2.0:** Add Jenkins and ArgoCD support, cross-provider benchmarks, deployment intelligence, and advanced policy controls.

### Technical Risks
- **Diagnosis quality risk:** Incorrect or low-confidence root-cause inference on ambiguous logs.
  - Mitigation: confidence thresholds, evidence-first outputs, and benchmark-driven iteration.
- **Provider dependency risk:** GitHub API limits/outages reduce tool reliability.
  - Mitigation: caching, backoff/retry policies, and graceful degradation modes.
- **Performance risk:** Large logs and high concurrency increase latency and cost.
  - Mitigation: bounded parsing, streaming analysis, and workload-aware timeouts.
- **Security risk:** Sensitive data leakage from pipeline logs.
  - Mitigation: deterministic redaction pipelines, encrypted storage, and strict retention windows.
- **Operational risk:** Misuse of rerun actions.
  - Mitigation: explicit action reason, role checks, and complete auditability.
