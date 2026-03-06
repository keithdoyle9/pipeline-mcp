package githubapi

import "time"

type WorkflowRun struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	DisplayTitle string     `json:"display_title"`
	Status       string     `json:"status"`
	Conclusion   string     `json:"conclusion"`
	HTMLURL      string     `json:"html_url"`
	HeadSHA      string     `json:"head_sha"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RunStartedAt *time.Time `json:"run_started_at"`
}

type Step struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	Number      int        `json:"number"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
}

type Job struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	HeadBranch  string     `json:"head_branch"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	HTMLURL     string     `json:"html_url"`
	CheckRunURL string     `json:"check_run_url"`
	RunnerID    int64      `json:"runner_id"`
	Steps       []Step     `json:"steps"`
}

type listJobsResponse struct {
	TotalCount int   `json:"total_count"`
	Jobs       []Job `json:"jobs"`
}

type listRunsResponse struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
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
	ID         int64           `json:"id"`
	Name       string          `json:"name"`
	HTMLURL    string          `json:"html_url"`
	DetailsURL string          `json:"details_url"`
	Status     string          `json:"status"`
	Conclusion string          `json:"conclusion"`
	Output     CheckRunOutput  `json:"output"`
	Deployment *DeploymentInfo `json:"deployment"`
}

type CheckRunOutput struct {
	Title          *string `json:"title"`
	Summary        *string `json:"summary"`
	Text           *string `json:"text"`
	AnnotationsURL string  `json:"annotations_url"`
}

type DeploymentInfo struct {
	ID                  int64  `json:"id"`
	Environment         string `json:"environment"`
	OriginalEnvironment string `json:"original_environment"`
}

type CheckRunAnnotation struct {
	Path            string `json:"path"`
	StartLine       int    `json:"start_line"`
	AnnotationLevel string `json:"annotation_level"`
	Title           string `json:"title"`
	Message         string `json:"message"`
	BlobHref        string `json:"blob_href"`
}

type BranchPolicy struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type deploymentBranchPoliciesResponse struct {
	TotalCount     int            `json:"total_count"`
	BranchPolicies []BranchPolicy `json:"branch_policies"`
}
