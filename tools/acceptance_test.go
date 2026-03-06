package tools

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/service"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
)

type acceptanceGitHubClient struct {
	getRunFn             func(ctx context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error)
	listRunJobsFn        func(ctx context.Context, owner, repo string, runID int64) ([]githubapi.Job, error)
	downloadRunLogsFn    func(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error)
	listRepositoryRunsFn func(ctx context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error)
	getCheckRunFn        func(ctx context.Context, owner, repo string, checkRunID int64) (*githubapi.CheckRun, error)
	getAnnotationsFn     func(ctx context.Context, owner, repo string, checkRunID int64) ([]githubapi.CheckRunAnnotation, error)
	listPoliciesFn       func(ctx context.Context, owner, repo, environment string) ([]githubapi.BranchPolicy, error)
	rerunFn              func(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error
}

func (m *acceptanceGitHubClient) GetRun(ctx context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error) {
	if m.getRunFn == nil {
		return nil, nil
	}
	return m.getRunFn(ctx, owner, repo, runID)
}

func (m *acceptanceGitHubClient) ListRunJobs(ctx context.Context, owner, repo string, runID int64) ([]githubapi.Job, error) {
	if m.listRunJobsFn == nil {
		return nil, nil
	}
	return m.listRunJobsFn(ctx, owner, repo, runID)
}

func (m *acceptanceGitHubClient) DownloadRunLogs(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error) {
	if m.downloadRunLogsFn == nil {
		return "", nil
	}
	return m.downloadRunLogsFn(ctx, owner, repo, runID, maxBytes)
}

func (m *acceptanceGitHubClient) ListRepositoryRuns(ctx context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error) {
	if m.listRepositoryRunsFn == nil {
		return nil, nil
	}
	return m.listRepositoryRunsFn(ctx, owner, repo, opts, maxRuns)
}

func (m *acceptanceGitHubClient) GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*githubapi.CheckRun, error) {
	if m.getCheckRunFn == nil {
		return nil, nil
	}
	return m.getCheckRunFn(ctx, owner, repo, checkRunID)
}

func (m *acceptanceGitHubClient) GetCheckRunAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]githubapi.CheckRunAnnotation, error) {
	if m.getAnnotationsFn == nil {
		return nil, nil
	}
	return m.getAnnotationsFn(ctx, owner, repo, checkRunID)
}

func (m *acceptanceGitHubClient) ListDeploymentBranchPolicies(ctx context.Context, owner, repo, environment string) ([]githubapi.BranchPolicy, error) {
	if m.listPoliciesFn == nil {
		return nil, nil
	}
	return m.listPoliciesFn(ctx, owner, repo, environment)
}

func (m *acceptanceGitHubClient) Rerun(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error {
	if m.rerunFn == nil {
		return nil
	}
	return m.rerunFn(ctx, owner, repo, runID, failedJobsOnly)
}

type acceptanceAuditStore struct {
	events []domain.AuditEvent
}

func (m *acceptanceAuditStore) Append(_ context.Context, event domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestAcceptanceAC1DiagnoseFailureReturnsDiagnosticAndRecommendations(t *testing.T) {
	deps := newAcceptanceDependencies(t, &acceptanceGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 55, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/55"}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return []githubapi.Job{{Name: "test", Conclusion: "failure"}}, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500", nil
		},
	}, &acceptanceAuditStore{}, true)

	_, out, err := deps.diagnoseFailure(context.Background(), nil, DiagnoseFailureInput{
		RunURL: "https://github.com/acme/app/actions/runs/55",
	})
	if err != nil {
		t.Fatalf("diagnoseFailure() error = %v", err)
	}
	if out.Error != nil {
		t.Fatalf("unexpected tool error: %+v", out.Error)
	}
	if out.Diagnostic == nil || out.Diagnostic.FailureCategory == "" {
		t.Fatalf("expected diagnostic output, got %+v", out.Diagnostic)
	}
	if out.Diagnostic.Confidence <= 0 {
		t.Fatalf("expected confidence > 0, got %f", out.Diagnostic.Confidence)
	}
	if len(out.Diagnostic.EvidenceRefs) == 0 {
		t.Fatal("expected evidence references")
	}
	if len(out.Recommendations) == 0 {
		t.Fatal("expected fix recommendations")
	}
}

func TestAcceptanceAC2DiagnoseFailureMapsLogUnavailable(t *testing.T) {
	deps := newAcceptanceDependencies(t, &acceptanceGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 55, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/55"}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return []githubapi.Job{{Name: "test", Conclusion: "failure"}}, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "", githubapi.ErrLogsUnavailable
		},
	}, &acceptanceAuditStore{}, true)

	_, out, err := deps.diagnoseFailure(context.Background(), nil, DiagnoseFailureInput{
		RunURL: "https://github.com/acme/app/actions/runs/55",
	})
	if err != nil {
		t.Fatalf("diagnoseFailure() error = %v", err)
	}
	if out.Error == nil {
		t.Fatal("expected tool error")
	}
	if out.Error.Code != domain.ErrorCodeLogUnavailable {
		t.Fatalf("expected %s, got %s", domain.ErrorCodeLogUnavailable, out.Error.Code)
	}
}

func TestAcceptanceAC3AnalyzeFlakyTestsReturnsFrequencyRecencyConfidence(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	deps := newAcceptanceDependencies(t, &acceptanceGitHubClient{
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return []githubapi.WorkflowRun{
				{ID: 1, Name: "ci", Conclusion: "failure", UpdatedAt: now.Add(-2 * time.Hour)},
				{ID: 2, Name: "ci", Conclusion: "failure", UpdatedAt: now.Add(-1 * time.Hour)},
				{ID: 3, Name: "ci", Conclusion: "success", UpdatedAt: now.Add(-30 * time.Minute)},
			}, nil
		},
		downloadRunLogsFn: func(_ context.Context, _ string, _ string, runID int64, _ int64) (string, error) {
			if runID == 1 || runID == 2 {
				return "--- FAIL: TestCheckout", nil
			}
			return "", nil
		},
	}, &acceptanceAuditStore{}, true)

	_, out, err := deps.analyzeFlakyTests(context.Background(), nil, AnalyzeFlakyTestsInput{
		Repository:   "acme/app",
		LookbackDays: 14,
	})
	if err != nil {
		t.Fatalf("analyzeFlakyTests() error = %v", err)
	}
	if out.Error != nil {
		t.Fatalf("unexpected tool error: %+v", out.Error)
	}
	if out.Report == nil || len(out.Report.TopFlaky) == 0 {
		t.Fatalf("expected flaky report, got %+v", out.Report)
	}
	top := out.Report.TopFlaky[0]
	if top.FailureFrequency < 2 {
		t.Fatalf("expected failure frequency >= 2, got %d", top.FailureFrequency)
	}
	if top.Recency == "" || top.Recency == "unknown" {
		t.Fatalf("expected recency to be populated, got %q", top.Recency)
	}
	if top.Confidence <= 0 {
		t.Fatalf("expected confidence > 0, got %f", top.Confidence)
	}
}

func TestAcceptanceAC4RerunRequiresReasonAndWritesAuditEvent(t *testing.T) {
	auditStore := &acceptanceAuditStore{}
	deps := newAcceptanceDependencies(t, &acceptanceGitHubClient{
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}, auditStore, false)

	_, invalid, err := deps.rerun(context.Background(), nil, RerunInput{
		Repository:     "acme/app",
		RunID:          99,
		FailedJobsOnly: true,
	})
	if err != nil {
		t.Fatalf("rerun() validation error = %v", err)
	}
	if invalid.Error == nil || invalid.Error.Code != domain.ErrorCodeInvalidInput {
		t.Fatalf("expected INVALID_INPUT, got %+v", invalid.Error)
	}

	_, out, err := deps.rerun(context.Background(), nil, RerunInput{
		Repository:     "acme/app",
		RunID:          99,
		FailedJobsOnly: true,
		Reason:         "retry flaky network failure",
	})
	if err != nil {
		t.Fatalf("rerun() error = %v", err)
	}
	if out.Error != nil {
		t.Fatalf("unexpected tool error: %+v", out.Error)
	}
	if out.Result == nil || out.Result.Scope != "failed_jobs_only" {
		t.Fatalf("expected rerun scope failed_jobs_only, got %+v", out.Result)
	}
	if len(auditStore.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditStore.events))
	}
	if auditStore.events[0].Reason != "retry flaky network failure" {
		t.Fatalf("unexpected audit reason: %+v", auditStore.events[0])
	}
}

func TestAcceptanceAC5ComparePerformanceReturnsBaselineCurrentAndFailureBreakdown(t *testing.T) {
	from := time.Date(2026, 3, 6, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)

	deps := newAcceptanceDependencies(t, &acceptanceGitHubClient{
		listRepositoryRunsFn: func(_ context.Context, _ string, _ string, opts githubapi.ListRunsOptions, _ int) ([]githubapi.WorkflowRun, error) {
			switch opts.Created {
			case "2026-03-06T11:00:00Z..2026-03-06T12:00:00Z":
				currentStart := from.Add(5 * time.Minute)
				currentEnd := to.Add(-10 * time.Minute)
				failedStart := from.Add(10 * time.Minute)
				failedEnd := to.Add(-5 * time.Minute)
				return []githubapi.WorkflowRun{
					{ID: 2, Name: "ci", Conclusion: "success", CreatedAt: from, RunStartedAt: &currentStart, UpdatedAt: currentEnd, HeadSHA: "sha-2"},
					{ID: 3, Name: "ci", Conclusion: "failure", CreatedAt: from.Add(2 * time.Minute), RunStartedAt: &failedStart, UpdatedAt: failedEnd, HeadSHA: "sha-3"},
				}, nil
			case "2026-03-06T10:00:00Z..2026-03-06T11:00:00Z":
				baselineStart := from.Add(-55 * time.Minute)
				baselineEnd := from.Add(-15 * time.Minute)
				return []githubapi.WorkflowRun{
					{ID: 1, Name: "ci", Conclusion: "failure", CreatedAt: from.Add(-60 * time.Minute), RunStartedAt: &baselineStart, UpdatedAt: baselineEnd, HeadSHA: "sha-1"},
				}, nil
			default:
				t.Fatalf("unexpected created range: %s", opts.Created)
				return nil, nil
			}
		},
	}, &acceptanceAuditStore{}, true)

	_, out, err := deps.comparePerformance(context.Background(), nil, ComparePerformanceInput{
		Repository: "acme/app",
		Workflow:   "ci",
		From:       from.Format(time.RFC3339),
		To:         to.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("comparePerformance() error = %v", err)
	}
	if out.Error != nil {
		t.Fatalf("unexpected tool error: %+v", out.Error)
	}
	if out.Snapshot == nil {
		t.Fatal("expected performance snapshot")
	}
	if out.Snapshot.Baseline.TotalRuns == 0 || out.Snapshot.Current.TotalRuns == 0 {
		t.Fatalf("expected both windows to contain runs, got %+v", out.Snapshot)
	}
	if len(out.Snapshot.Baseline.FailureBreakdown) == 0 || len(out.Snapshot.Current.FailureBreakdown) == 0 {
		t.Fatalf("expected failure breakdown in both windows, got %+v", out.Snapshot)
	}
	if out.Snapshot.Current.QueueTimeMS == 0 || out.Snapshot.Current.MedianDurationMS == 0 {
		t.Fatalf("expected queue time and duration metrics, got %+v", out.Snapshot.Current)
	}
}

func newAcceptanceDependencies(t *testing.T, client *acceptanceGitHubClient, auditStore *acceptanceAuditStore, disableMutations bool) Dependencies {
	t.Helper()

	collector := telemetry.NewCollector("")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		ServerName:          "pipeline-mcp",
		Version:             "test",
		GitHubAPIBaseURL:    "https://api.github.com",
		DisableMutations:    disableMutations,
		MaxLogBytes:         20 * 1024 * 1024,
		DefaultLookbackDays: 14,
		MaxHistoricalRuns:   100,
		Actor:               "pipeline-mcp",
	}

	return Dependencies{
		Service:   service.New(cfg, client, auditStore, collector, logger),
		Telemetry: collector,
		Logger:    logger,
	}
}
