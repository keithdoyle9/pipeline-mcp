package gitlabapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

type mockProviderClient struct {
	getPipelineFn      func(context.Context, string, int64) (*Pipeline, error)
	listPipelineJobsFn func(context.Context, string, int64) ([]Job, error)
	downloadJobTraceFn func(context.Context, string, int64, int64) (string, error)
	listProjectPipesFn func(context.Context, string, ListPipelinesOptions, int) ([]Pipeline, error)
	retryPipelineFn    func(context.Context, string, int64) error
}

func (m *mockProviderClient) GetPipeline(ctx context.Context, projectPath string, pipelineID int64) (*Pipeline, error) {
	return m.getPipelineFn(ctx, projectPath, pipelineID)
}

func (m *mockProviderClient) ListPipelineJobs(ctx context.Context, projectPath string, pipelineID int64) ([]Job, error) {
	return m.listPipelineJobsFn(ctx, projectPath, pipelineID)
}

func (m *mockProviderClient) DownloadJobTrace(ctx context.Context, projectPath string, jobID int64, maxBytes int64) (string, error) {
	return m.downloadJobTraceFn(ctx, projectPath, jobID, maxBytes)
}

func (m *mockProviderClient) ListProjectPipelines(ctx context.Context, projectPath string, opts ListPipelinesOptions, maxRuns int) ([]Pipeline, error) {
	return m.listProjectPipesFn(ctx, projectPath, opts, maxRuns)
}

func (m *mockProviderClient) RetryPipeline(ctx context.Context, projectPath string, pipelineID int64) error {
	return m.retryPipelineFn(ctx, projectPath, pipelineID)
}

func TestProviderAdapterParseRepository(t *testing.T) {
	adapter := NewProviderAdapter(&mockProviderClient{}, "https://gitlab.example.com/api/v4")
	repository, err := adapter.ParseRepository("group/subgroup/app")
	if err != nil {
		t.Fatalf("ParseRepository() error = %v", err)
	}
	if repository != "group/subgroup/app" {
		t.Fatalf("unexpected repository %q", repository)
	}
}

func TestProviderAdapterParseRunURL(t *testing.T) {
	adapter := NewProviderAdapter(&mockProviderClient{}, "https://gitlab.example.com/gitlab/api/v4")
	locator, err := adapter.ParseRunURL("https://gitlab.example.com/gitlab/group/subgroup/app/-/pipelines/77")
	if err != nil {
		t.Fatalf("ParseRunURL() error = %v", err)
	}
	if locator.Repository != "group/subgroup/app" || locator.RunID != 77 {
		t.Fatalf("unexpected locator %+v", locator)
	}
}

func TestProviderAdapterListRepositoryRunsTranslatesCreatedRangeAndStatus(t *testing.T) {
	client := &mockProviderClient{
		listProjectPipesFn: func(_ context.Context, projectPath string, opts ListPipelinesOptions, maxRuns int) ([]Pipeline, error) {
			if projectPath != "group/subgroup/app" {
				t.Fatalf("unexpected project path %q", projectPath)
			}
			if opts.CreatedAfter != "2026-03-06T11:00:00Z" || opts.CreatedBefore != "2026-03-06T12:00:00Z" {
				t.Fatalf("unexpected created range %+v", opts)
			}
			if opts.Status != "failed" {
				t.Fatalf("expected failed status, got %q", opts.Status)
			}
			if opts.OrderBy != "updated_at" || opts.Sort != "desc" {
				t.Fatalf("unexpected ordering %+v", opts)
			}
			if maxRuns != 1 {
				t.Fatalf("expected maxRuns 1, got %d", maxRuns)
			}
			return []Pipeline{{ID: 99, Name: "ci", Status: "failed"}}, nil
		},
	}
	adapter := NewProviderAdapter(client, "https://gitlab.example.com/api/v4")

	runs, err := adapter.ListRepositoryRuns(context.Background(), "group/subgroup/app", providers.ListRunsOptions{
		Created: "2026-03-06T11:00:00Z..2026-03-06T12:00:00Z",
		Status:  "failure",
		PerPage: 1,
	}, 1)
	if err != nil {
		t.Fatalf("ListRepositoryRuns() error = %v", err)
	}
	if len(runs) != 1 || runs[0].Conclusion != "failure" {
		t.Fatalf("unexpected runs %+v", runs)
	}
}

func TestProviderAdapterDownloadRunLogsOrdersFailedJobsFirstAndAppliesMaxBytes(t *testing.T) {
	client := &mockProviderClient{
		listPipelineJobsFn: func(_ context.Context, _ string, _ int64) ([]Job, error) {
			now := time.Now().UTC()
			return []Job{
				{ID: 2, Name: "lint", Status: "success", StartedAt: &now},
				{ID: 1, Name: "test", Status: "failed", StartedAt: &now},
			}, nil
		},
		downloadJobTraceFn: func(_ context.Context, _ string, jobID int64, _ int64) (string, error) {
			switch jobID {
			case 1:
				return "--- FAIL: TestCheckout", nil
			case 2:
				return "ok", nil
			default:
				return "", ErrLogsUnavailable
			}
		},
	}
	adapter := NewProviderAdapter(client, "https://gitlab.example.com/api/v4")

	logs, err := adapter.DownloadRunLogs(context.Background(), "group/app", 10, 48)
	if err != nil {
		t.Fatalf("DownloadRunLogs() error = %v", err)
	}
	if !strings.Contains(logs, "test") {
		t.Fatalf("expected failed job trace in logs, got %q", logs)
	}
	if strings.Contains(logs, "lint") {
		t.Fatalf("expected maxBytes truncation before lint job, got %q", logs)
	}
}

func TestProviderAdapterDownloadRunLogsReturnsUnavailableWhenNoTraceExists(t *testing.T) {
	client := &mockProviderClient{
		listPipelineJobsFn: func(_ context.Context, _ string, _ int64) ([]Job, error) {
			return []Job{{ID: 1, Name: "test", Status: "failed"}}, nil
		},
		downloadJobTraceFn: func(_ context.Context, _ string, _ int64, _ int64) (string, error) {
			return "", ErrLogsUnavailable
		},
	}
	adapter := NewProviderAdapter(client, "https://gitlab.example.com/api/v4")

	_, err := adapter.DownloadRunLogs(context.Background(), "group/app", 10, 1024)
	if !errors.Is(err, ErrLogsUnavailable) {
		t.Fatalf("expected ErrLogsUnavailable, got %v", err)
	}
}

func TestProviderAdapterMapError(t *testing.T) {
	adapter := NewProviderAdapter(&mockProviderClient{}, "https://gitlab.example.com/api/v4")
	testCases := []struct {
		err      error
		wantCode string
	}{
		{err: ErrUnauthorized, wantCode: domain.ErrorCodeUnauthorized},
		{err: ErrNotFound, wantCode: domain.ErrorCodeUnauthorized},
		{err: ErrLogsUnavailable, wantCode: domain.ErrorCodeLogUnavailable},
		{err: ErrRateLimited, wantCode: domain.ErrorCodeRateLimited},
		{err: ErrProviderUnavailable, wantCode: domain.ErrorCodeProviderUnavailable},
		{err: ErrFullRerunUnsupported, wantCode: domain.ErrorCodeInvalidInput},
	}
	for _, tc := range testCases {
		toolErr := adapter.MapError(tc.err)
		if toolErr == nil || toolErr.Code != tc.wantCode {
			t.Fatalf("MapError(%v) = %+v, want code %s", tc.err, toolErr, tc.wantCode)
		}
	}
}

func TestProviderAdapterRerunRejectsFullRun(t *testing.T) {
	client := &mockProviderClient{
		retryPipelineFn: func(context.Context, string, int64) error {
			t.Fatal("expected rerun rejection before API call")
			return nil
		},
	}
	adapter := NewProviderAdapter(client, "https://gitlab.example.com/api/v4")

	err := adapter.Rerun(context.Background(), "group/app", 12, false)
	if !errors.Is(err, ErrFullRerunUnsupported) {
		t.Fatalf("expected ErrFullRerunUnsupported, got %v", err)
	}
}
