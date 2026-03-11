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
	"github.com/keithdoyle9/pipeline-mcp/internal/gitlabapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
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

type acceptanceGitLabClient struct {
	getPipelineFn      func(ctx context.Context, projectPath string, pipelineID int64) (*gitlabapi.Pipeline, error)
	listPipelineJobsFn func(ctx context.Context, projectPath string, pipelineID int64) ([]gitlabapi.Job, error)
	downloadJobTraceFn func(ctx context.Context, projectPath string, jobID int64, maxBytes int64) (string, error)
	listPipelinesFn    func(ctx context.Context, projectPath string, opts gitlabapi.ListPipelinesOptions, maxRuns int) ([]gitlabapi.Pipeline, error)
	retryPipelineFn    func(ctx context.Context, projectPath string, pipelineID int64) error
}

func (m *acceptanceGitLabClient) GetPipeline(ctx context.Context, projectPath string, pipelineID int64) (*gitlabapi.Pipeline, error) {
	if m.getPipelineFn == nil {
		return nil, nil
	}
	return m.getPipelineFn(ctx, projectPath, pipelineID)
}

func (m *acceptanceGitLabClient) ListPipelineJobs(ctx context.Context, projectPath string, pipelineID int64) ([]gitlabapi.Job, error) {
	if m.listPipelineJobsFn == nil {
		return nil, nil
	}
	return m.listPipelineJobsFn(ctx, projectPath, pipelineID)
}

func (m *acceptanceGitLabClient) DownloadJobTrace(ctx context.Context, projectPath string, jobID int64, maxBytes int64) (string, error) {
	if m.downloadJobTraceFn == nil {
		return "", nil
	}
	return m.downloadJobTraceFn(ctx, projectPath, jobID, maxBytes)
}

func (m *acceptanceGitLabClient) ListProjectPipelines(ctx context.Context, projectPath string, opts gitlabapi.ListPipelinesOptions, maxRuns int) ([]gitlabapi.Pipeline, error) {
	if m.listPipelinesFn == nil {
		return nil, nil
	}
	return m.listPipelinesFn(ctx, projectPath, opts, maxRuns)
}

func (m *acceptanceGitLabClient) RetryPipeline(ctx context.Context, projectPath string, pipelineID int64) error {
	if m.retryPipelineFn == nil {
		return nil
	}
	return m.retryPipelineFn(ctx, projectPath, pipelineID)
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

func TestAcceptanceGitLabAC1DiagnoseFailureReturnsDiagnosticAndRecommendations(t *testing.T) {
	deps := newAcceptanceDependenciesWithAdapters(
		t,
		[]providers.Adapter{
			githubapi.NewProviderAdapter(&acceptanceGitHubClient{}, "https://api.github.com"),
			gitlabapi.NewProviderAdapter(&acceptanceGitLabClient{
				getPipelineFn: func(context.Context, string, int64) (*gitlabapi.Pipeline, error) {
					return &gitlabapi.Pipeline{ID: 55, Name: "ci", WebURL: "https://gitlab.example.com/group/app/-/pipelines/55"}, nil
				},
				listPipelineJobsFn: func(context.Context, string, int64) ([]gitlabapi.Job, error) {
					return []gitlabapi.Job{{ID: 4, Name: "test", Status: "failed"}}, nil
				},
				downloadJobTraceFn: func(context.Context, string, int64, int64) (string, error) {
					return "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500", nil
				},
			}, "https://gitlab.example.com/api/v4"),
		},
		&acceptanceAuditStore{},
		true,
	)

	_, out, err := deps.diagnoseFailure(context.Background(), nil, DiagnoseFailureInput{
		Provider:   "gitlab_ci",
		Repository: "group/app",
		RunID:      55,
	})
	if err != nil {
		t.Fatalf("diagnoseFailure() error = %v", err)
	}
	if out.Error != nil {
		t.Fatalf("unexpected tool error: %+v", out.Error)
	}
	if out.Diagnostic == nil || out.Diagnostic.FailureCategory == "" || len(out.Recommendations) == 0 {
		t.Fatalf("expected diagnostic output, got %+v %+v", out.Diagnostic, out.Recommendations)
	}
}

func TestAcceptanceGitLabAC2DiagnoseFailureMapsLogUnavailable(t *testing.T) {
	deps := newAcceptanceDependenciesWithAdapters(
		t,
		[]providers.Adapter{
			githubapi.NewProviderAdapter(&acceptanceGitHubClient{}, "https://api.github.com"),
			gitlabapi.NewProviderAdapter(&acceptanceGitLabClient{
				getPipelineFn: func(context.Context, string, int64) (*gitlabapi.Pipeline, error) {
					return &gitlabapi.Pipeline{ID: 55, Name: "ci", WebURL: "https://gitlab.example.com/group/app/-/pipelines/55"}, nil
				},
				listPipelineJobsFn: func(context.Context, string, int64) ([]gitlabapi.Job, error) {
					return []gitlabapi.Job{{ID: 4, Name: "test", Status: "failed"}}, nil
				},
				downloadJobTraceFn: func(context.Context, string, int64, int64) (string, error) {
					return "", gitlabapi.ErrLogsUnavailable
				},
			}, "https://gitlab.example.com/api/v4"),
		},
		&acceptanceAuditStore{},
		true,
	)

	_, out, err := deps.diagnoseFailure(context.Background(), nil, DiagnoseFailureInput{
		Provider:   "gitlab_ci",
		Repository: "group/app",
		RunID:      55,
	})
	if err != nil {
		t.Fatalf("diagnoseFailure() error = %v", err)
	}
	if out.Error == nil || out.Error.Code != domain.ErrorCodeLogUnavailable {
		t.Fatalf("expected LOG_UNAVAILABLE, got %+v", out.Error)
	}
}

func TestAcceptanceGitLabAC3AnalyzeFlakyTestsReturnsFrequencyRecencyConfidence(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	deps := newAcceptanceDependenciesWithAdapters(
		t,
		[]providers.Adapter{
			githubapi.NewProviderAdapter(&acceptanceGitHubClient{}, "https://api.github.com"),
			gitlabapi.NewProviderAdapter(&acceptanceGitLabClient{
				listPipelinesFn: func(context.Context, string, gitlabapi.ListPipelinesOptions, int) ([]gitlabapi.Pipeline, error) {
					return []gitlabapi.Pipeline{
						{ID: 1, Name: "ci", Status: "failed", UpdatedAt: now.Add(-2 * time.Hour)},
						{ID: 2, Name: "ci", Status: "failed", UpdatedAt: now.Add(-1 * time.Hour)},
						{ID: 3, Name: "ci", Status: "success", UpdatedAt: now.Add(-30 * time.Minute)},
					}, nil
				},
				listPipelineJobsFn: func(context.Context, string, int64) ([]gitlabapi.Job, error) {
					return []gitlabapi.Job{{ID: 5, Name: "test", Status: "failed"}}, nil
				},
				downloadJobTraceFn: func(_ context.Context, _ string, jobID int64, _ int64) (string, error) {
					if jobID == 5 {
						return "--- FAIL: TestCheckout", nil
					}
					return "", nil
				},
			}, "https://gitlab.example.com/api/v4"),
		},
		&acceptanceAuditStore{},
		true,
	)

	_, out, err := deps.analyzeFlakyTests(context.Background(), nil, AnalyzeFlakyTestsInput{
		Provider:     "gitlab_ci",
		Repository:   "group/app",
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
}

func TestAcceptanceGitLabAC4RerunSupportsFailedJobsOnlyAndRejectsFullRun(t *testing.T) {
	auditStore := &acceptanceAuditStore{}
	deps := newAcceptanceDependenciesWithAdapters(
		t,
		[]providers.Adapter{
			githubapi.NewProviderAdapter(&acceptanceGitHubClient{}, "https://api.github.com"),
			gitlabapi.NewProviderAdapter(&acceptanceGitLabClient{
				retryPipelineFn: func(context.Context, string, int64) error { return nil },
			}, "https://gitlab.example.com/api/v4"),
		},
		auditStore,
		false,
	)

	_, invalid, err := deps.rerun(context.Background(), nil, RerunInput{
		Provider:       "gitlab_ci",
		Repository:     "group/app",
		RunID:          99,
		FailedJobsOnly: false,
		Reason:         "retry all jobs",
	})
	if err != nil {
		t.Fatalf("rerun() validation error = %v", err)
	}
	if invalid.Error == nil || invalid.Error.Code != domain.ErrorCodeInvalidInput {
		t.Fatalf("expected INVALID_INPUT, got %+v", invalid.Error)
	}

	_, out, err := deps.rerun(context.Background(), nil, RerunInput{
		Provider:       "gitlab_ci",
		Repository:     "group/app",
		RunID:          99,
		FailedJobsOnly: true,
		Reason:         "retry failed jobs",
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
	if len(auditStore.events) != 2 {
		t.Fatalf("expected 2 audit events, got %d", len(auditStore.events))
	}
}

func TestAcceptanceGitLabAC5ComparePerformanceReturnsBaselineCurrentAndFailureBreakdown(t *testing.T) {
	from := time.Date(2026, 3, 6, 11, 0, 0, 0, time.UTC)
	to := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	deps := newAcceptanceDependenciesWithAdapters(
		t,
		[]providers.Adapter{
			githubapi.NewProviderAdapter(&acceptanceGitHubClient{}, "https://api.github.com"),
			gitlabapi.NewProviderAdapter(&acceptanceGitLabClient{
				listPipelinesFn: func(_ context.Context, _ string, opts gitlabapi.ListPipelinesOptions, _ int) ([]gitlabapi.Pipeline, error) {
					switch {
					case opts.CreatedAfter == "2026-03-06T11:00:00Z" && opts.CreatedBefore == "2026-03-06T12:00:00Z":
						currentStart := from.Add(5 * time.Minute)
						currentEnd := to.Add(-10 * time.Minute)
						failedStart := from.Add(10 * time.Minute)
						failedEnd := to.Add(-5 * time.Minute)
						return []gitlabapi.Pipeline{
							{ID: 2, Name: "ci", Status: "success", CreatedAt: from, StartedAt: &currentStart, FinishedAt: &currentEnd, UpdatedAt: currentEnd, SHA: "sha-2"},
							{ID: 3, Name: "ci", Status: "failed", CreatedAt: from.Add(2 * time.Minute), StartedAt: &failedStart, FinishedAt: &failedEnd, UpdatedAt: failedEnd, SHA: "sha-3"},
						}, nil
					case opts.CreatedAfter == "2026-03-06T10:00:00Z" && opts.CreatedBefore == "2026-03-06T11:00:00Z":
						baselineStart := from.Add(-55 * time.Minute)
						baselineEnd := from.Add(-15 * time.Minute)
						return []gitlabapi.Pipeline{
							{ID: 1, Name: "ci", Status: "failed", CreatedAt: from.Add(-60 * time.Minute), StartedAt: &baselineStart, FinishedAt: &baselineEnd, UpdatedAt: baselineEnd, SHA: "sha-1"},
						}, nil
					default:
						t.Fatalf("unexpected created range after=%s before=%s", opts.CreatedAfter, opts.CreatedBefore)
						return nil, nil
					}
				},
			}, "https://gitlab.example.com/api/v4"),
		},
		&acceptanceAuditStore{},
		true,
	)

	_, out, err := deps.comparePerformance(context.Background(), nil, ComparePerformanceInput{
		Provider:   "gitlab_ci",
		Repository: "group/app",
		Workflow:   "ci",
		From:       from.Format(time.RFC3339),
		To:         to.Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("comparePerformance() error = %v", err)
	}
	if out.Error != nil || out.Snapshot == nil {
		t.Fatalf("expected performance snapshot, got error=%+v snapshot=%+v", out.Error, out.Snapshot)
	}
}

func newAcceptanceDependencies(t *testing.T, client *acceptanceGitHubClient, auditStore *acceptanceAuditStore, disableMutations bool) Dependencies {
	t.Helper()

	return newAcceptanceDependenciesWithAdapters(t, []providers.Adapter{githubapi.NewProviderAdapter(client, "https://api.github.com")}, auditStore, disableMutations)
}

func newAcceptanceDependenciesWithAdapters(t *testing.T, adapters []providers.Adapter, auditStore *acceptanceAuditStore, disableMutations bool) Dependencies {
	t.Helper()

	collector := telemetry.NewCollector("")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &config.Config{
		ServerName:          "pipeline-mcp",
		Version:             "test",
		GitHubAPIBaseURL:    "https://api.github.com",
		GitLabAPIBaseURL:    "https://gitlab.com/api/v4",
		DisableMutations:    disableMutations,
		MaxLogBytes:         20 * 1024 * 1024,
		DefaultLookbackDays: 14,
		MaxHistoricalRuns:   100,
		Actor:               "pipeline-mcp",
	}

	return Dependencies{
		Service:   service.New(cfg, mustAcceptanceRegistry(t, adapters...), auditStore, collector, logger),
		Telemetry: collector,
		Logger:    logger,
	}
}

func mustAcceptanceRegistry(t *testing.T, adapters ...providers.Adapter) *providers.Registry {
	t.Helper()

	registry, err := providers.NewRegistry(adapters[0].ProviderID(), adapters...)
	if err != nil {
		t.Fatalf("providers.NewRegistry() error = %v", err)
	}
	return registry
}
