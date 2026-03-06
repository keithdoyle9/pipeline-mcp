# Acceptance Criteria Mapping (AC-1 to AC-5)

- **AC-1**: `pipeline.diagnose_failure` returns `failure_diagnostic` and `fix_recommendations` with confidence and evidence refs.
- **AC-2**: Log access failures map to structured `LOG_UNAVAILABLE` (plus `UNAUTHORIZED`/`RATE_LIMITED` as applicable).
- **AC-3**: `pipeline.analyze_flaky_tests` returns top flaky tests with frequency, recency, and confidence over configurable lookback.
- **AC-4**: `pipeline.rerun` requires explicit scope (`failed_jobs_only` boolean maps to scope), required `reason`, and writes audit event.
- **AC-5**: `pipeline.compare_performance` returns baseline/current windows for queue time, median duration, success rate, and failure breakdown.
