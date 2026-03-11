package gitlabapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

type providerClient interface {
	GetPipeline(ctx context.Context, projectPath string, pipelineID int64) (*Pipeline, error)
	ListPipelineJobs(ctx context.Context, projectPath string, pipelineID int64) ([]Job, error)
	DownloadJobTrace(ctx context.Context, projectPath string, jobID int64, maxBytes int64) (string, error)
	ListProjectPipelines(ctx context.Context, projectPath string, opts ListPipelinesOptions, maxRuns int) ([]Pipeline, error)
	RetryPipeline(ctx context.Context, projectPath string, pipelineID int64) error
}

type ProviderAdapter struct {
	client     providerClient
	apiBaseURL string
}

func NewProviderAdapter(client providerClient, apiBaseURL string) *ProviderAdapter {
	return &ProviderAdapter{
		client:     client,
		apiBaseURL: strings.TrimRight(strings.TrimSpace(apiBaseURL), "/"),
	}
}

func (a *ProviderAdapter) ProviderID() string {
	return domain.ProviderGitLab
}

func (a *ProviderAdapter) ParseRepository(repository string) (string, error) {
	return ParseProjectPath(repository)
}

func (a *ProviderAdapter) ParseRunURL(raw string) (*providers.RunLocator, error) {
	locator, err := ParseRunURLForBase(raw, a.apiBaseURL)
	if err != nil {
		return nil, err
	}
	return &providers.RunLocator{
		Repository: locator.Repository(),
		RunID:      locator.RunID,
		RunURL:     locator.RunURL,
	}, nil
}

func (a *ProviderAdapter) ParseCheckRunURL(string) (int64, error) {
	return 0, ErrCheckRunUnsupported
}

func (a *ProviderAdapter) RunURL(repository string, runID int64) string {
	return RunURLForBase(repository, runID, a.apiBaseURL)
}

func (a *ProviderAdapter) GetRun(ctx context.Context, repository string, runID int64) (*providers.Run, error) {
	pipeline, err := a.client.GetPipeline(ctx, repository, runID)
	if err != nil || pipeline == nil {
		return nil, err
	}
	return toProviderRun(pipeline), nil
}

func (a *ProviderAdapter) ListRunJobs(ctx context.Context, repository string, runID int64) ([]providers.Job, error) {
	jobs, err := a.client.ListPipelineJobs(ctx, repository, runID)
	if err != nil {
		return nil, err
	}
	return toProviderJobs(jobs), nil
}

func (a *ProviderAdapter) DownloadRunLogs(ctx context.Context, repository string, runID int64, maxBytes int64) (string, error) {
	jobs, err := a.client.ListPipelineJobs(ctx, repository, runID)
	if err != nil {
		return "", err
	}
	if len(jobs) == 0 {
		return "", ErrLogsUnavailable
	}

	ordered := append([]Job(nil), jobs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		ri := jobLogPriority(ordered[i])
		rj := jobLogPriority(ordered[j])
		if ri == rj {
			return ordered[i].ID < ordered[j].ID
		}
		return ri < rj
	})

	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}

	var out bytes.Buffer
	for _, job := range ordered {
		trace, err := a.client.DownloadJobTrace(ctx, repository, job.ID, maxBytes)
		if err != nil {
			if errors.Is(err, ErrLogsUnavailable) {
				continue
			}
			return "", err
		}
		trace = strings.TrimSpace(trace)
		if trace == "" {
			continue
		}

		chunk := fmt.Sprintf("=== job: %s (%d) ===\n%s\n", firstNonEmpty(job.Name, job.Stage, "job"), job.ID, trace)
		if !appendLimited(&out, []byte(chunk), maxBytes) {
			break
		}
	}

	if out.Len() == 0 {
		return "", ErrLogsUnavailable
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

func (a *ProviderAdapter) ListRepositoryRuns(ctx context.Context, repository string, opts providers.ListRunsOptions, maxRuns int) ([]providers.Run, error) {
	listOpts := ListPipelinesOptions{
		PerPage: opts.PerPage,
		Page:    opts.Page,
		Status:  translatePipelineStatus(opts.Status),
		OrderBy: "updated_at",
		Sort:    "desc",
	}
	createdAfter, createdBefore, err := parseCreatedRange(opts.Created)
	if err != nil {
		return nil, err
	}
	listOpts.CreatedAfter = createdAfter
	listOpts.CreatedBefore = createdBefore

	pipelines, err := a.client.ListProjectPipelines(ctx, repository, listOpts, maxRuns)
	if err != nil {
		return nil, err
	}
	return toProviderRuns(pipelines), nil
}

func (a *ProviderAdapter) GetCheckRun(context.Context, string, int64) (*providers.CheckRun, error) {
	return nil, nil
}

func (a *ProviderAdapter) GetCheckRunAnnotations(context.Context, string, int64) ([]providers.CheckRunAnnotation, error) {
	return nil, nil
}

func (a *ProviderAdapter) ListDeploymentBranchPolicies(context.Context, string, string) ([]providers.BranchPolicy, error) {
	return nil, nil
}

func (a *ProviderAdapter) Rerun(ctx context.Context, repository string, runID int64, failedJobsOnly bool) error {
	if !failedJobsOnly {
		return ErrFullRerunUnsupported
	}
	return a.client.RetryPipeline(ctx, repository, runID)
}

func (a *ProviderAdapter) IsLogsUnavailable(err error) bool {
	return errors.Is(err, ErrLogsUnavailable)
}

func (a *ProviderAdapter) MapError(err error) *domain.ToolError {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrFullRerunUnsupported) {
		return domain.NewToolError(
			domain.ErrorCodeInvalidInput,
			"GitLab rerun supports failed or canceled jobs only.",
			"Set failed_jobs_only=true when provider=\"gitlab_ci\".",
			false,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrWriteTokenRequired) {
		return domain.NewToolError(
			domain.ErrorCodeUnauthorized,
			"GitLab API authorization failed.",
			"Verify token scopes: read_api for read tools and api for rerun.",
			false,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrNotFound) {
		return domain.NewToolError(
			domain.ErrorCodeUnauthorized,
			"Run not found or access denied.",
			"Confirm the pipeline URL is correct and provide a token that can read this project.",
			false,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrRateLimited) {
		return domain.NewToolError(
			domain.ErrorCodeRateLimited,
			"GitLab API rate limit exceeded.",
			"Retry after backoff or increase API quota/token capacity.",
			true,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrLogsUnavailable) {
		return domain.NewToolError(
			domain.ErrorCodeLogUnavailable,
			"Run logs are unavailable.",
			"Check project permissions and job log retention settings.",
			true,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrProviderUnavailable) {
		return domain.NewToolError(
			domain.ErrorCodeProviderUnavailable,
			"GitLab API is unavailable.",
			"Retry with exponential backoff and check provider status.",
			true,
			map[string]any{"error": err.Error()},
		)
	}
	return domain.NewToolError(domain.ErrorCodeInternal, "Unexpected internal error.", "Check server logs and retry.", true, map[string]any{"error": err.Error()})
}

func toProviderRun(pipeline *Pipeline) *providers.Run {
	if pipeline == nil {
		return nil
	}
	updatedAt := pipeline.UpdatedAt
	if pipeline.FinishedAt != nil && !pipeline.FinishedAt.IsZero() {
		updatedAt = *pipeline.FinishedAt
	}
	return &providers.Run{
		ID:           pipeline.ID,
		Name:         pipeline.Name,
		DisplayTitle: pipeline.Name,
		Status:       pipeline.Status,
		Conclusion:   normalizeConclusion(pipeline.Status),
		RunURL:       pipeline.WebURL,
		HeadSHA:      pipeline.SHA,
		CreatedAt:    pipeline.CreatedAt,
		UpdatedAt:    updatedAt,
		RunStartedAt: pipeline.StartedAt,
	}
}

func toProviderRuns(pipelines []Pipeline) []providers.Run {
	out := make([]providers.Run, 0, len(pipelines))
	for _, pipeline := range pipelines {
		if run := toProviderRun(&pipeline); run != nil {
			out = append(out, *run)
		}
	}
	return out
}

func toProviderJobs(jobs []Job) []providers.Job {
	out := make([]providers.Job, 0, len(jobs))
	for _, job := range jobs {
		out = append(out, providers.Job{
			ID:          job.ID,
			Name:        job.Name,
			Status:      job.Status,
			Conclusion:  normalizeConclusion(job.Status),
			HeadBranch:  job.Ref,
			StartedAt:   job.StartedAt,
			CompletedAt: job.FinishedAt,
			RunURL:      job.WebURL,
		})
	}
	return out
}

func parseCreatedRange(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", nil
	}
	parts := strings.Split(raw, "..")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("created range must be start..end")
	}
	start, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[0]))
	if err != nil {
		return "", "", fmt.Errorf("parse created range start: %w", err)
	}
	end, err := time.Parse(time.RFC3339, strings.TrimSpace(parts[1]))
	if err != nil {
		return "", "", fmt.Errorf("parse created range end: %w", err)
	}
	return start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339), nil
}

func normalizeConclusion(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return "success"
	case "failed":
		return "failure"
	case "canceled":
		return "cancelled"
	case "skipped":
		return "skipped"
	default:
		return ""
	}
}

func translatePipelineStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failure":
		return "failed"
	default:
		return strings.TrimSpace(status)
	}
}

func jobLogPriority(job Job) int {
	switch strings.ToLower(strings.TrimSpace(job.Status)) {
	case "failed", "canceled":
		return 0
	default:
		return 1
	}
}

func appendLimited(dst *bytes.Buffer, chunk []byte, limit int64) bool {
	remaining := limit - int64(dst.Len())
	if remaining <= 0 {
		return false
	}
	if int64(len(chunk)) > remaining {
		dst.Write(chunk[:remaining])
		return false
	}
	dst.Write(chunk)
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
