package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/service"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
)

type caseFixture struct {
	Name             string                         `json:"name"`
	RunURL           string                         `json:"run_url"`
	Logs             string                         `json:"logs"`
	ExpectedCategory string                         `json:"expected_category"`
	Jobs             []githubapi.Job                `json:"jobs,omitempty"`
	CheckRun         *githubapi.CheckRun            `json:"check_run,omitempty"`
	Annotations      []githubapi.CheckRunAnnotation `json:"annotations,omitempty"`
	BranchPolicies   []githubapi.BranchPolicy       `json:"branch_policies,omitempty"`
}

type benchmarkReport struct {
	Cases        int     `json:"cases"`
	Top1Accuracy float64 `json:"top1_accuracy"`
	Top3Accuracy float64 `json:"top3_accuracy"`
}

type benchmarkGitHubClient struct {
	fixture caseFixture
}

func (b benchmarkGitHubClient) GetRun(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
	return &githubapi.WorkflowRun{
		ID:         1,
		Name:       "benchmark",
		HTMLURL:    b.fixture.RunURL,
		Conclusion: "failure",
	}, nil
}

func (b benchmarkGitHubClient) ListRunJobs(context.Context, string, string, int64) ([]githubapi.Job, error) {
	if len(b.fixture.Jobs) > 0 {
		return b.fixture.Jobs, nil
	}
	return []githubapi.Job{{Name: "job", Conclusion: "failure"}}, nil
}

func (b benchmarkGitHubClient) DownloadRunLogs(context.Context, string, string, int64, int64) (string, error) {
	return b.fixture.Logs, nil
}

func (b benchmarkGitHubClient) ListRepositoryRuns(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
	return nil, nil
}

func (b benchmarkGitHubClient) GetCheckRun(context.Context, string, string, int64) (*githubapi.CheckRun, error) {
	return b.fixture.CheckRun, nil
}

func (b benchmarkGitHubClient) GetCheckRunAnnotations(context.Context, string, string, int64) ([]githubapi.CheckRunAnnotation, error) {
	return b.fixture.Annotations, nil
}

func (b benchmarkGitHubClient) ListDeploymentBranchPolicies(context.Context, string, string, string) ([]githubapi.BranchPolicy, error) {
	return b.fixture.BranchPolicies, nil
}

func (b benchmarkGitHubClient) Rerun(context.Context, string, string, int64, bool) error {
	return nil
}

type noopAuditStore struct{}

func (noopAuditStore) Append(context.Context, domain.AuditEvent) error {
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "benchmark failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	payload, err := os.ReadFile("testdata/benchmarks/historical_failures.json")
	if err != nil {
		return err
	}
	var fixtures []caseFixture
	if err := json.Unmarshal(payload, &fixtures); err != nil {
		return err
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("no benchmark fixtures found")
	}

	correct := 0
	for _, fixture := range fixtures {
		cfg := &config.Config{
			ServerName:          "pipeline-mcp-benchmark",
			Version:             "benchmark",
			GitHubAPIBaseURL:    "https://api.github.com",
			DisableMutations:    true,
			MaxLogBytes:         20 * 1024 * 1024,
			DefaultLookbackDays: 14,
			MaxHistoricalRuns:   100,
			Actor:               "benchmark",
		}
		svc := service.New(
			cfg,
			benchmarkGitHubClient{fixture: fixture},
			noopAuditStore{},
			telemetry.NewCollector(""),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		diagnostic, _, toolErr := svc.DiagnoseFailure(context.Background(), service.RunReference{RunURL: fixture.RunURL}, cfg.MaxLogBytes)
		if toolErr != nil {
			return fmt.Errorf("fixture %q failed: %s", fixture.Name, toolErr.Message)
		}
		if diagnostic.FailureCategory == fixture.ExpectedCategory {
			correct++
		}
	}

	acc := float64(correct) / float64(len(fixtures))
	report := benchmarkReport{Cases: len(fixtures), Top1Accuracy: acc, Top3Accuracy: acc}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}
