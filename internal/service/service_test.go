package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
)

type mockGitHubClient struct {
	getRunFn             func(ctx context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error)
	listRunJobsFn        func(ctx context.Context, owner, repo string, runID int64) ([]githubapi.Job, error)
	downloadRunLogsFn    func(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error)
	listRepositoryRunsFn func(ctx context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error)
	getCheckRunFn        func(ctx context.Context, owner, repo string, checkRunID int64) (*githubapi.CheckRun, error)
	getAnnotationsFn     func(ctx context.Context, owner, repo string, checkRunID int64) ([]githubapi.CheckRunAnnotation, error)
	listPoliciesFn       func(ctx context.Context, owner, repo, environment string) ([]githubapi.BranchPolicy, error)
	rerunFn              func(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error
}

func (m *mockGitHubClient) GetRun(ctx context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error) {
	if m.getRunFn == nil {
		return nil, nil
	}
	return m.getRunFn(ctx, owner, repo, runID)
}
func (m *mockGitHubClient) ListRunJobs(ctx context.Context, owner, repo string, runID int64) ([]githubapi.Job, error) {
	if m.listRunJobsFn == nil {
		return nil, nil
	}
	return m.listRunJobsFn(ctx, owner, repo, runID)
}
func (m *mockGitHubClient) DownloadRunLogs(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error) {
	if m.downloadRunLogsFn == nil {
		return "", nil
	}
	return m.downloadRunLogsFn(ctx, owner, repo, runID, maxBytes)
}
func (m *mockGitHubClient) ListRepositoryRuns(ctx context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error) {
	if m.listRepositoryRunsFn == nil {
		return nil, nil
	}
	return m.listRepositoryRunsFn(ctx, owner, repo, opts, maxRuns)
}
func (m *mockGitHubClient) GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*githubapi.CheckRun, error) {
	if m.getCheckRunFn == nil {
		return nil, nil
	}
	return m.getCheckRunFn(ctx, owner, repo, checkRunID)
}
func (m *mockGitHubClient) GetCheckRunAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]githubapi.CheckRunAnnotation, error) {
	if m.getAnnotationsFn == nil {
		return nil, nil
	}
	return m.getAnnotationsFn(ctx, owner, repo, checkRunID)
}
func (m *mockGitHubClient) ListDeploymentBranchPolicies(ctx context.Context, owner, repo, environment string) ([]githubapi.BranchPolicy, error) {
	if m.listPoliciesFn == nil {
		return nil, nil
	}
	return m.listPoliciesFn(ctx, owner, repo, environment)
}
func (m *mockGitHubClient) Rerun(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error {
	if m.rerunFn == nil {
		return nil
	}
	return m.rerunFn(ctx, owner, repo, runID, failedJobsOnly)
}

type memoryAuditStore struct {
	events []domain.AuditEvent
}

func (m *memoryAuditStore) Append(_ context.Context, event domain.AuditEvent) error {
	m.events = append(m.events, event)
	return nil
}

func TestDiagnoseFailureSuccess(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	mock := &mockGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 55, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/55", HeadSHA: "abc123", CreatedAt: now.Add(-10 * time.Minute), UpdatedAt: now}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return []githubapi.Job{{Name: "test", Conclusion: "failure"}}, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500", nil
		},
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return nil, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := newTestService(mock, false)
	diagnostic, recs, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{RunURL: "https://github.com/acme/app/actions/runs/55"}, 0)
	if toolErr != nil {
		t.Fatalf("unexpected tool error: %+v", toolErr)
	}
	if diagnostic == nil || diagnostic.FailureCategory == "" {
		t.Fatal("expected diagnostic")
	}
	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}
}

func TestDiagnoseFailureLogsUnavailable(t *testing.T) {
	mock := &mockGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 55, Name: "ci"}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return nil, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "", githubapi.ErrLogsUnavailable
		},
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return nil, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := newTestService(mock, false)
	_, _, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{RunURL: "https://github.com/acme/app/actions/runs/55"}, 0)
	if toolErr == nil {
		t.Fatal("expected tool error")
	}
	if toolErr.Code != domain.ErrorCodeLogUnavailable {
		t.Fatalf("expected %s, got %s", domain.ErrorCodeLogUnavailable, toolErr.Code)
	}
}

func TestAnalyzeFlakyTests(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	mock := &mockGitHubClient{
		getRunFn:      func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) { return nil, nil },
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) { return nil, nil },
		downloadRunLogsFn: func(_ context.Context, _ string, _ string, runID int64, _ int64) (string, error) {
			if runID == 1 || runID == 2 {
				return "--- FAIL: TestCheckout", nil
			}
			return "", errors.New("missing")
		},
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return []githubapi.WorkflowRun{
				{ID: 1, Conclusion: "failure", UpdatedAt: now.Add(-2 * time.Hour)},
				{ID: 2, Conclusion: "failure", UpdatedAt: now.Add(-1 * time.Hour)},
				{ID: 3, Conclusion: "success", UpdatedAt: now.Add(-30 * time.Minute)},
			}, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := newTestService(mock, false)
	svc.now = func() time.Time { return now }
	report, toolErr := svc.AnalyzeFlakyTests(context.Background(), "acme/app", 14, "")
	if toolErr != nil {
		t.Fatalf("unexpected error: %+v", toolErr)
	}
	if len(report.TopFlaky) == 0 {
		t.Fatal("expected flaky tests in report")
	}
}

func TestRerunRequiresReasonAndAudit(t *testing.T) {
	auditStore := &memoryAuditStore{}
	cfg := testConfig(false)
	cfg.Actor = "ci-oncall"

	mock := &mockGitHubClient{
		getRunFn:          func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) { return nil, nil },
		listRunJobsFn:     func(context.Context, string, string, int64) ([]githubapi.Job, error) { return nil, nil },
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) { return "", nil },
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return nil, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := &Service{
		cfg:       cfg,
		github:    mock,
		audit:     auditStore,
		telemetry: telemetry.NewCollector(""),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:       time.Now,
	}

	if _, err := svc.Rerun(context.Background(), "acme/app", 99, true, ""); err == nil {
		t.Fatal("expected reason validation error")
	}

	result, toolErr := svc.Rerun(context.Background(), "acme/app", 99, true, "retry flaky network failure")
	if toolErr != nil {
		t.Fatalf("unexpected rerun error: %+v", toolErr)
	}
	if result.Scope != "failed_jobs_only" {
		t.Fatalf("expected failed_jobs_only scope, got %s", result.Scope)
	}
	if len(auditStore.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditStore.events))
	}
}

func TestComparePerformance(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	mock := &mockGitHubClient{
		getRunFn:      func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) { return nil, nil },
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) { return nil, nil },
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "", nil
		},
		listRepositoryRunsFn: func(_ context.Context, _ string, _ string, opts githubapi.ListRunsOptions, _ int) ([]githubapi.WorkflowRun, error) {
			if opts.Created == "2026-03-06T11:00:00Z..2026-03-06T12:00:00Z" {
				start := now.Add(-55 * time.Minute)
				end := now.Add(-10 * time.Minute)
				return []githubapi.WorkflowRun{{ID: 2, Name: "ci", Conclusion: "success", CreatedAt: now.Add(-60 * time.Minute), RunStartedAt: &start, UpdatedAt: end}}, nil
			}
			start := now.Add(-115 * time.Minute)
			end := now.Add(-70 * time.Minute)
			return []githubapi.WorkflowRun{{ID: 1, Name: "ci", Conclusion: "failure", CreatedAt: now.Add(-120 * time.Minute), RunStartedAt: &start, UpdatedAt: end}}, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := newTestService(mock, false)
	from := now.Add(-1 * time.Hour)
	to := now
	snapshot, toolErr := svc.ComparePerformance(context.Background(), "acme/app", "ci", from, to)
	if toolErr != nil {
		t.Fatalf("unexpected error: %+v", toolErr)
	}
	if snapshot.Current.TotalRuns == 0 {
		t.Fatal("expected current runs")
	}
	if snapshot.Baseline.TotalRuns == 0 {
		t.Fatal("expected baseline runs")
	}
}

func TestGetRunMapsNotFoundToUnauthorized(t *testing.T) {
	mock := &mockGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return nil, githubapi.ErrNotFound
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return nil, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			return "", nil
		},
		listRepositoryRunsFn: func(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
			return nil, nil
		},
		rerunFn: func(context.Context, string, string, int64, bool) error { return nil },
	}

	svc := newTestService(mock, false)
	_, toolErr := svc.GetRun(context.Background(), RunReference{RunURL: "https://github.com/acme/app/actions/runs/55"})
	if toolErr == nil {
		t.Fatal("expected tool error")
	}
	if toolErr.Code != domain.ErrorCodeUnauthorized {
		t.Fatalf("expected %s, got %s", domain.ErrorCodeUnauthorized, toolErr.Code)
	}
}

func TestDiagnoseFailureFromEnvironmentProtectionAnnotations(t *testing.T) {
	mock := &mockGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 55, Name: "Release Validation", HTMLURL: "https://github.com/acme/app/actions/runs/55"}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return []githubapi.Job{{
				ID:          7,
				Name:        "ios-release-candidate",
				Conclusion:  "failure",
				HeadBranch:  "main",
				CheckRunURL: "https://api.github.com/repos/acme/app/check-runs/7",
				Steps:       nil,
			}}, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			t.Fatal("expected metadata-based diagnosis before reading logs")
			return "", nil
		},
		getCheckRunFn: func(context.Context, string, string, int64) (*githubapi.CheckRun, error) {
			return &githubapi.CheckRun{
				ID: 7,
				Deployment: &githubapi.DeploymentInfo{
					Environment:         "ios-ci",
					OriginalEnvironment: "ios-ci",
				},
			}, nil
		},
		getAnnotationsFn: func(context.Context, string, string, int64) ([]githubapi.CheckRunAnnotation, error) {
			return []githubapi.CheckRunAnnotation{
				{
					Path:      ".github",
					StartLine: 1,
					Message:   "Branch \"main\" is not allowed to deploy to ios-ci due to environment protection rules.",
				},
				{
					Path:      ".github",
					StartLine: 1,
					Message:   "The deployment was rejected or didn't satisfy other protection rules.",
				},
			}, nil
		},
		listPoliciesFn: func(context.Context, string, string, string) ([]githubapi.BranchPolicy, error) {
			return []githubapi.BranchPolicy{{Name: "release/*", Type: "branch"}}, nil
		},
	}

	svc := newTestService(mock, false)
	diagnostic, recs, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{RunURL: "https://github.com/acme/app/actions/runs/55"}, 0)
	if toolErr != nil {
		t.Fatalf("unexpected tool error: %+v", toolErr)
	}
	if diagnostic == nil {
		t.Fatal("expected diagnostic")
	}
	if diagnostic.FailureCategory != "config_error" {
		t.Fatalf("expected config_error, got %s", diagnostic.FailureCategory)
	}
	if diagnostic.Confidence < 0.9 {
		t.Fatalf("expected high confidence, got %f", diagnostic.Confidence)
	}
	if !strings.Contains(diagnostic.SuspectedRootCause, "release/*") {
		t.Fatalf("expected branch policy in root cause, got %q", diagnostic.SuspectedRootCause)
	}
	if len(recs) < 2 {
		t.Fatalf("expected recommendations, got %d", len(recs))
	}
}

func TestDiagnoseFailureFromApprovalGateAnnotations(t *testing.T) {
	mock := &mockGitHubClient{
		getRunFn: func(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
			return &githubapi.WorkflowRun{ID: 88, Name: "deploy", HTMLURL: "https://github.com/acme/app/actions/runs/88"}, nil
		},
		listRunJobsFn: func(context.Context, string, string, int64) ([]githubapi.Job, error) {
			return []githubapi.Job{{
				ID:          11,
				Name:        "deploy-production",
				Conclusion:  "failure",
				HeadBranch:  "main",
				CheckRunURL: "https://api.github.com/repos/acme/app/check-runs/11",
				Steps:       nil,
			}}, nil
		},
		downloadRunLogsFn: func(context.Context, string, string, int64, int64) (string, error) {
			t.Fatal("expected metadata-based diagnosis before reading logs")
			return "", nil
		},
		getCheckRunFn: func(context.Context, string, string, int64) (*githubapi.CheckRun, error) {
			return &githubapi.CheckRun{
				ID: 11,
				Deployment: &githubapi.DeploymentInfo{
					Environment:         "production",
					OriginalEnvironment: "production",
				},
			}, nil
		},
		getAnnotationsFn: func(context.Context, string, string, int64) ([]githubapi.CheckRunAnnotation, error) {
			return []githubapi.CheckRunAnnotation{
				{
					Path:      ".github",
					StartLine: 1,
					Message:   "Deployment to production is awaiting review from required reviewers.",
				},
				{
					Path:      ".github",
					StartLine: 1,
					Message:   "This job cannot continue until the required reviewers approve the deployment.",
				},
			}, nil
		},
	}

	svc := newTestService(mock, false)
	diagnostic, recs, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{RunURL: "https://github.com/acme/app/actions/runs/88"}, 0)
	if toolErr != nil {
		t.Fatalf("unexpected tool error: %+v", toolErr)
	}
	if diagnostic == nil {
		t.Fatal("expected diagnostic")
	}
	if diagnostic.FailureCategory != "config_error" {
		t.Fatalf("expected config_error, got %s", diagnostic.FailureCategory)
	}
	if diagnostic.Confidence < 0.9 {
		t.Fatalf("expected high confidence, got %f", diagnostic.Confidence)
	}
	if !strings.Contains(strings.ToLower(diagnostic.SuspectedRootCause), "required reviewer approval") {
		t.Fatalf("expected approval gate root cause, got %q", diagnostic.SuspectedRootCause)
	}
	if len(recs) < 2 {
		t.Fatalf("expected recommendations, got %d", len(recs))
	}
}

func TestGetRunResolvesLatestFailedRunFromRepository(t *testing.T) {
	mock := &mockGitHubClient{
		listRepositoryRunsFn: func(_ context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error) {
			if owner != "acme" || repo != "app" {
				t.Fatalf("unexpected repository %s/%s", owner, repo)
			}
			if opts.Status != "failure" {
				t.Fatalf("expected status filter failure, got %q", opts.Status)
			}
			if opts.PerPage != 1 || maxRuns != 1 {
				t.Fatalf("expected single-run lookup, got per_page=%d maxRuns=%d", opts.PerPage, maxRuns)
			}
			return []githubapi.WorkflowRun{{
				ID:         144,
				Name:       "ci",
				HTMLURL:    "https://github.com/acme/app/actions/runs/144",
				Conclusion: "failure",
			}}, nil
		},
		getRunFn: func(_ context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error) {
			if owner != "acme" || repo != "app" || runID != 144 {
				t.Fatalf("unexpected run lookup %s/%s#%d", owner, repo, runID)
			}
			return &githubapi.WorkflowRun{ID: 144, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/144", Conclusion: "failure"}, nil
		},
		listRunJobsFn: func(_ context.Context, _ string, _ string, runID int64) ([]githubapi.Job, error) {
			if runID != 144 {
				t.Fatalf("unexpected run id for jobs %d", runID)
			}
			return []githubapi.Job{{Name: "test", Conclusion: "failure"}}, nil
		},
	}

	svc := newTestService(mock, false)
	run, toolErr := svc.GetRun(context.Background(), RunReference{Repository: "acme/app"})
	if toolErr != nil {
		t.Fatalf("unexpected tool error: %+v", toolErr)
	}
	if run == nil || run.RunID != 144 {
		t.Fatalf("expected latest failed run 144, got %+v", run)
	}
}

func TestDiagnoseFailureResolvesLatestFailedRunFromRepository(t *testing.T) {
	mock := &mockGitHubClient{
		listRepositoryRunsFn: func(_ context.Context, owner, repo string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error) {
			if owner != "acme" || repo != "app" {
				t.Fatalf("unexpected repository %s/%s", owner, repo)
			}
			if opts.Status != "failure" {
				t.Fatalf("expected status filter failure, got %q", opts.Status)
			}
			if opts.PerPage != 1 || maxRuns != 1 {
				t.Fatalf("expected single-run lookup, got per_page=%d maxRuns=%d", opts.PerPage, maxRuns)
			}
			return []githubapi.WorkflowRun{{
				ID:         145,
				Name:       "ci",
				HTMLURL:    "https://github.com/acme/app/actions/runs/145",
				Conclusion: "failure",
			}}, nil
		},
		getRunFn: func(_ context.Context, owner, repo string, runID int64) (*githubapi.WorkflowRun, error) {
			if owner != "acme" || repo != "app" || runID != 145 {
				t.Fatalf("unexpected run lookup %s/%s#%d", owner, repo, runID)
			}
			return &githubapi.WorkflowRun{ID: 145, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/145", Conclusion: "failure"}, nil
		},
		listRunJobsFn: func(_ context.Context, _ string, _ string, runID int64) ([]githubapi.Job, error) {
			if runID != 145 {
				t.Fatalf("unexpected run id for jobs %d", runID)
			}
			return []githubapi.Job{{Name: "test", Conclusion: "failure"}}, nil
		},
		downloadRunLogsFn: func(_ context.Context, _ string, _ string, runID int64, _ int64) (string, error) {
			if runID != 145 {
				t.Fatalf("unexpected run id for logs %d", runID)
			}
			return "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500", nil
		},
	}

	svc := newTestService(mock, false)
	diagnostic, recs, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{Repository: "acme/app"}, 0)
	if toolErr != nil {
		t.Fatalf("unexpected tool error: %+v", toolErr)
	}
	if diagnostic == nil || diagnostic.FailureCategory == "" {
		t.Fatalf("expected diagnostic, got %+v", diagnostic)
	}
	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}
}

func TestDiagnoseFailureRepositoryOnlyReturnsHelpfulErrorWhenNoFailuresExist(t *testing.T) {
	mock := &mockGitHubClient{
		listRepositoryRunsFn: func(_ context.Context, _ string, _ string, opts githubapi.ListRunsOptions, maxRuns int) ([]githubapi.WorkflowRun, error) {
			if opts.Status != "failure" || opts.PerPage != 1 || maxRuns != 1 {
				t.Fatalf("unexpected latest-failure lookup opts=%+v maxRuns=%d", opts, maxRuns)
			}
			return nil, nil
		},
	}

	svc := newTestService(mock, false)
	_, _, toolErr := svc.DiagnoseFailure(context.Background(), RunReference{Repository: "acme/app"}, 0)
	if toolErr == nil {
		t.Fatal("expected tool error")
	}
	if toolErr.Code != domain.ErrorCodeInvalidInput {
		t.Fatalf("expected %s, got %s", domain.ErrorCodeInvalidInput, toolErr.Code)
	}
	if !strings.Contains(strings.ToLower(toolErr.Message), "no failed workflow runs") {
		t.Fatalf("unexpected message: %q", toolErr.Message)
	}
}

func newTestService(client GitHubClient, disableMutations bool) *Service {
	cfg := testConfig(disableMutations)
	return &Service{
		cfg:       cfg,
		github:    client,
		audit:     &memoryAuditStore{},
		telemetry: telemetry.NewCollector(""),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		now:       time.Now,
	}
}

func testConfig(disableMutations bool) *config.Config {
	return &config.Config{
		ServerName:          "pipeline-mcp",
		Version:             "test",
		GitHubAPIBaseURL:    "https://api.github.com",
		DisableMutations:    disableMutations,
		MaxLogBytes:         20 * 1024 * 1024,
		DefaultLookbackDays: 14,
		MaxHistoricalRuns:   100,
		Actor:               "pipeline-mcp",
	}
}
