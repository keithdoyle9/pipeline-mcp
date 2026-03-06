# Acceptance Criteria Mapping (AC-1 to AC-5)

- **AC-1**: `pipeline.diagnose_failure` returns `failure_diagnostic` and `fix_recommendations` with confidence and evidence refs.
  Verified by `TestAcceptanceAC1DiagnoseFailureReturnsDiagnosticAndRecommendations` in `tools/acceptance_test.go`.
- **AC-2**: Log access failures map to structured `LOG_UNAVAILABLE` (plus `UNAUTHORIZED`/`RATE_LIMITED` as applicable).
  Verified by `TestAcceptanceAC2DiagnoseFailureMapsLogUnavailable` in `tools/acceptance_test.go`.
- **AC-3**: `pipeline.analyze_flaky_tests` returns top flaky tests with frequency, recency, and confidence over configurable lookback.
  Verified by `TestAcceptanceAC3AnalyzeFlakyTestsReturnsFrequencyRecencyConfidence` in `tools/acceptance_test.go`.
- **AC-4**: `pipeline.rerun` requires explicit scope (`failed_jobs_only` boolean maps to scope), required `reason`, and writes audit event.
  Verified by `TestAcceptanceAC4RerunRequiresReasonAndWritesAuditEvent` in `tools/acceptance_test.go`.
- **AC-5**: `pipeline.compare_performance` returns baseline/current windows for queue time, median duration, success rate, and failure breakdown.
  Verified by `TestAcceptanceAC5ComparePerformanceReturnsBaselineCurrentAndFailureBreakdown` in `tools/acceptance_test.go`.
