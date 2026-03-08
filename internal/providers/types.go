package providers

import (
	"context"
	"fmt"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
)

type Adapter interface {
	ProviderID() string
	ParseRepository(repository string) (owner string, repo string, err error)
	ParseRunURL(raw string) (*RunLocator, error)
	ParseCheckRunURL(raw string) (int64, error)
	RunURL(owner, repo string, runID int64) string
	GetRun(ctx context.Context, owner, repo string, runID int64) (*Run, error)
	ListRunJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error)
	DownloadRunLogs(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error)
	ListRepositoryRuns(ctx context.Context, owner, repo string, opts ListRunsOptions, maxRuns int) ([]Run, error)
	GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*CheckRun, error)
	GetCheckRunAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]CheckRunAnnotation, error)
	ListDeploymentBranchPolicies(ctx context.Context, owner, repo, environment string) ([]BranchPolicy, error)
	Rerun(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error
	IsLogsUnavailable(err error) bool
	MapError(err error) *domain.ToolError
}

type RunLocator struct {
	Owner  string
	Repo   string
	RunID  int64
	RunURL string
}

func (r RunLocator) Repository() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Repo)
}

type Run struct {
	ID           int64
	Name         string
	DisplayTitle string
	Status       string
	Conclusion   string
	RunURL       string
	HeadSHA      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	RunStartedAt *time.Time
}

type Step struct {
	Name        string
	Status      string
	Conclusion  string
	Number      int
	StartedAt   *time.Time
	CompletedAt *time.Time
}

type Job struct {
	ID          int64
	Name        string
	Status      string
	Conclusion  string
	HeadBranch  string
	StartedAt   *time.Time
	CompletedAt *time.Time
	RunURL      string
	CheckRunURL string
	RunnerID    int64
	Steps       []Step
}

type ListRunsOptions struct {
	PerPage int
	Page    int
	Created string
	Branch  string
	Event   string
	Status  string
}

type CheckRun struct {
	ID         int64
	Name       string
	RunURL     string
	DetailsURL string
	Status     string
	Conclusion string
	Output     CheckRunOutput
	Deployment *DeploymentInfo
}

type CheckRunOutput struct {
	Title          *string
	Summary        *string
	Text           *string
	AnnotationsURL string
}

type DeploymentInfo struct {
	ID                  int64
	Environment         string
	OriginalEnvironment string
}

type CheckRunAnnotation struct {
	Path            string
	StartLine       int
	AnnotationLevel string
	Title           string
	Message         string
	BlobHref        string
}

type BranchPolicy struct {
	ID   int64
	Name string
	Type string
}
