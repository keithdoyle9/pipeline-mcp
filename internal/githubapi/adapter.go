package githubapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

type providerClient interface {
	GetRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRun, error)
	ListRunJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error)
	DownloadRunLogs(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error)
	ListRepositoryRuns(ctx context.Context, owner, repo string, opts ListRunsOptions, maxRuns int) ([]WorkflowRun, error)
	GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*CheckRun, error)
	GetCheckRunAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]CheckRunAnnotation, error)
	ListDeploymentBranchPolicies(ctx context.Context, owner, repo, environment string) ([]BranchPolicy, error)
	Rerun(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error
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
	return domain.ProviderGitHub
}

func (a *ProviderAdapter) ParseRepository(repository string) (string, error) {
	owner, repo, err := ParseRepository(repository)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%s", owner, repo), nil
}

func (a *ProviderAdapter) ParseRunURL(raw string) (*providers.RunLocator, error) {
	locator, err := ParseRunURL(raw)
	if err != nil {
		return nil, err
	}
	return &providers.RunLocator{
		Repository: locator.Repository(),
		RunID:      locator.RunID,
		RunURL:     locator.RunURL,
	}, nil
}

func (a *ProviderAdapter) ParseCheckRunURL(raw string) (int64, error) {
	if strings.TrimSpace(a.apiBaseURL) == "" {
		return ParseCheckRunURL(raw)
	}
	return ParseCheckRunURLForBase(raw, a.apiBaseURL)
}

func (a *ProviderAdapter) RunURL(repository string, runID int64) string {
	owner, repo, err := ParseRepository(repository)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", owner, repo, runID)
}

func (a *ProviderAdapter) GetRun(ctx context.Context, repository string, runID int64) (*providers.Run, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	run, err := a.client.GetRun(ctx, owner, repo, runID)
	if err != nil || run == nil {
		return nil, err
	}
	return toProviderRun(run), nil
}

func (a *ProviderAdapter) ListRunJobs(ctx context.Context, repository string, runID int64) ([]providers.Job, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	jobs, err := a.client.ListRunJobs(ctx, owner, repo, runID)
	if err != nil {
		return nil, err
	}
	return toProviderJobs(jobs), nil
}

func (a *ProviderAdapter) DownloadRunLogs(ctx context.Context, repository string, runID int64, maxBytes int64) (string, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return "", err
	}
	return a.client.DownloadRunLogs(ctx, owner, repo, runID, maxBytes)
}

func (a *ProviderAdapter) ListRepositoryRuns(ctx context.Context, repository string, opts providers.ListRunsOptions, maxRuns int) ([]providers.Run, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	runs, err := a.client.ListRepositoryRuns(ctx, owner, repo, ListRunsOptions{
		PerPage: opts.PerPage,
		Page:    opts.Page,
		Created: opts.Created,
		Branch:  opts.Branch,
		Event:   opts.Event,
		Status:  opts.Status,
	}, maxRuns)
	if err != nil {
		return nil, err
	}
	return toProviderRuns(runs), nil
}

func (a *ProviderAdapter) GetCheckRun(ctx context.Context, repository string, checkRunID int64) (*providers.CheckRun, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	checkRun, err := a.client.GetCheckRun(ctx, owner, repo, checkRunID)
	if err != nil || checkRun == nil {
		return nil, err
	}
	return toProviderCheckRun(checkRun), nil
}

func (a *ProviderAdapter) GetCheckRunAnnotations(ctx context.Context, repository string, checkRunID int64) ([]providers.CheckRunAnnotation, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	annotations, err := a.client.GetCheckRunAnnotations(ctx, owner, repo, checkRunID)
	if err != nil {
		return nil, err
	}
	return toProviderAnnotations(annotations), nil
}

func (a *ProviderAdapter) ListDeploymentBranchPolicies(ctx context.Context, repository, environment string) ([]providers.BranchPolicy, error) {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return nil, err
	}
	policies, err := a.client.ListDeploymentBranchPolicies(ctx, owner, repo, environment)
	if err != nil {
		return nil, err
	}
	return toProviderPolicies(policies), nil
}

func (a *ProviderAdapter) Rerun(ctx context.Context, repository string, runID int64, failedJobsOnly bool) error {
	owner, repo, err := a.ownerRepo(repository)
	if err != nil {
		return err
	}
	return a.client.Rerun(ctx, owner, repo, runID, failedJobsOnly)
}

func (a *ProviderAdapter) IsLogsUnavailable(err error) bool {
	return errors.Is(err, ErrLogsUnavailable)
}

func (a *ProviderAdapter) MapError(err error) *domain.ToolError {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrWriteTokenRequired) {
		return domain.NewToolError(domain.ErrorCodeUnauthorized, "GitHub API authorization failed.", "Verify token scopes: actions:read for read tools and actions:write for rerun.", false, map[string]any{"error": err.Error()})
	}
	if errors.Is(err, ErrNotFound) {
		return domain.NewToolError(
			domain.ErrorCodeUnauthorized,
			"Run not found or access denied.",
			"Confirm the run URL is correct and provide a token that can read this repository. For Claude Code, add the server with -e GITHUB_READ_TOKEN=... or -e GITHUB_TOKEN=....",
			false,
			map[string]any{"error": err.Error()},
		)
	}
	if errors.Is(err, ErrRateLimited) {
		return domain.NewToolError(domain.ErrorCodeRateLimited, "GitHub API rate limit exceeded.", "Retry after backoff or increase API quota/token capacity.", true, map[string]any{"error": err.Error()})
	}
	if errors.Is(err, ErrLogsUnavailable) {
		return domain.NewToolError(domain.ErrorCodeLogUnavailable, "Run logs are unavailable.", "Check repository permissions and log retention settings.", true, map[string]any{"error": err.Error()})
	}
	if errors.Is(err, ErrProviderUnavailable) {
		return domain.NewToolError(domain.ErrorCodeProviderUnavailable, "GitHub API is unavailable.", "Retry with exponential backoff and check provider status.", true, map[string]any{"error": err.Error()})
	}
	return domain.NewToolError(domain.ErrorCodeInternal, "Unexpected internal error.", "Check server logs and retry.", true, map[string]any{"error": err.Error()})
}

func (a *ProviderAdapter) ownerRepo(repository string) (string, string, error) {
	return ParseRepository(repository)
}

func toProviderRun(run *WorkflowRun) *providers.Run {
	if run == nil {
		return nil
	}
	return &providers.Run{
		ID:           run.ID,
		Name:         run.Name,
		DisplayTitle: run.DisplayTitle,
		Status:       run.Status,
		Conclusion:   run.Conclusion,
		RunURL:       run.HTMLURL,
		HeadSHA:      run.HeadSHA,
		CreatedAt:    run.CreatedAt,
		UpdatedAt:    run.UpdatedAt,
		RunStartedAt: run.RunStartedAt,
	}
}

func toProviderRuns(runs []WorkflowRun) []providers.Run {
	out := make([]providers.Run, 0, len(runs))
	for _, run := range runs {
		out = append(out, providers.Run{
			ID:           run.ID,
			Name:         run.Name,
			DisplayTitle: run.DisplayTitle,
			Status:       run.Status,
			Conclusion:   run.Conclusion,
			RunURL:       run.HTMLURL,
			HeadSHA:      run.HeadSHA,
			CreatedAt:    run.CreatedAt,
			UpdatedAt:    run.UpdatedAt,
			RunStartedAt: run.RunStartedAt,
		})
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
			Conclusion:  job.Conclusion,
			HeadBranch:  job.HeadBranch,
			StartedAt:   job.StartedAt,
			CompletedAt: job.CompletedAt,
			RunURL:      job.HTMLURL,
			CheckRunURL: job.CheckRunURL,
			RunnerID:    job.RunnerID,
			Steps:       toProviderSteps(job.Steps),
		})
	}
	return out
}

func toProviderSteps(steps []Step) []providers.Step {
	out := make([]providers.Step, 0, len(steps))
	for _, step := range steps {
		out = append(out, providers.Step{
			Name:        step.Name,
			Status:      step.Status,
			Conclusion:  step.Conclusion,
			Number:      step.Number,
			StartedAt:   step.StartedAt,
			CompletedAt: step.CompletedAt,
		})
	}
	return out
}

func toProviderCheckRun(checkRun *CheckRun) *providers.CheckRun {
	if checkRun == nil {
		return nil
	}
	return &providers.CheckRun{
		ID:         checkRun.ID,
		Name:       checkRun.Name,
		RunURL:     checkRun.HTMLURL,
		DetailsURL: checkRun.DetailsURL,
		Status:     checkRun.Status,
		Conclusion: checkRun.Conclusion,
		Output: providers.CheckRunOutput{
			Title:          checkRun.Output.Title,
			Summary:        checkRun.Output.Summary,
			Text:           checkRun.Output.Text,
			AnnotationsURL: checkRun.Output.AnnotationsURL,
		},
		Deployment: toProviderDeployment(checkRun.Deployment),
	}
}

func toProviderDeployment(deployment *DeploymentInfo) *providers.DeploymentInfo {
	if deployment == nil {
		return nil
	}
	return &providers.DeploymentInfo{
		ID:                  deployment.ID,
		Environment:         deployment.Environment,
		OriginalEnvironment: deployment.OriginalEnvironment,
	}
}

func toProviderAnnotations(annotations []CheckRunAnnotation) []providers.CheckRunAnnotation {
	out := make([]providers.CheckRunAnnotation, 0, len(annotations))
	for _, annotation := range annotations {
		out = append(out, providers.CheckRunAnnotation{
			Path:            annotation.Path,
			StartLine:       annotation.StartLine,
			AnnotationLevel: annotation.AnnotationLevel,
			Title:           annotation.Title,
			Message:         annotation.Message,
			BlobHref:        annotation.BlobHref,
		})
	}
	return out
}

func toProviderPolicies(policies []BranchPolicy) []providers.BranchPolicy {
	out := make([]providers.BranchPolicy, 0, len(policies))
	for _, policy := range policies {
		out = append(out, providers.BranchPolicy{
			ID:   policy.ID,
			Name: policy.Name,
			Type: policy.Type,
		})
	}
	return out
}
