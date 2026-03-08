package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/analysis"
	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
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
	Cases            int                   `json:"cases"`
	Top1Accuracy     float64               `json:"top1_accuracy"`
	Top3Accuracy     float64               `json:"top3_accuracy"`
	AverageLatencyMS float64               `json:"average_latency_ms"`
	P95LatencyMS     float64               `json:"p95_latency_ms"`
	Results          []benchmarkCaseResult `json:"results"`
}

type benchmarkCaseResult struct {
	Name             string   `json:"name"`
	ExpectedCategory string   `json:"expected_category"`
	TopCategory      string   `json:"top_category"`
	Top3Categories   []string `json:"top3_categories"`
	MatchedTop1      bool     `json:"matched_top1"`
	MatchedTop3      bool     `json:"matched_top3"`
	LatencyMS        float64  `json:"latency_ms"`
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

	results := make([]benchmarkCaseResult, 0, len(fixtures))
	for _, fixture := range fixtures {
		result, err := evaluateFixture(fixture)
		if err != nil {
			return err
		}
		results = append(results, result)
	}

	report := summarizeResults(results)

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func evaluateFixture(fixture caseFixture) (benchmarkCaseResult, error) {
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
		githubapi.NewProviderAdapter(benchmarkGitHubClient{fixture: fixture}, cfg.GitHubAPIBaseURL),
		noopAuditStore{},
		telemetry.NewCollector(""),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	start := time.Now()
	diagnostic, _, toolErr := svc.DiagnoseFailure(context.Background(), service.RunReference{RunURL: fixture.RunURL}, cfg.MaxLogBytes)
	latencyMS := float64(time.Since(start).Microseconds()) / 1000
	if toolErr != nil {
		return benchmarkCaseResult{}, fmt.Errorf("fixture %q failed: %s", fixture.Name, toolErr.Message)
	}

	top3 := topCategoriesForFixture(fixture, diagnostic)
	return benchmarkCaseResult{
		Name:             fixture.Name,
		ExpectedCategory: fixture.ExpectedCategory,
		TopCategory:      diagnostic.FailureCategory,
		Top3Categories:   top3,
		MatchedTop1:      diagnostic.FailureCategory == fixture.ExpectedCategory,
		MatchedTop3:      containsCategory(top3, fixture.ExpectedCategory),
		LatencyMS:        latencyMS,
	}, nil
}

func summarizeResults(results []benchmarkCaseResult) benchmarkReport {
	report := benchmarkReport{
		Cases:   len(results),
		Results: results,
	}
	if len(results) == 0 {
		return report
	}

	latencies := make([]float64, 0, len(results))
	var top1Correct int
	var top3Correct int
	for _, result := range results {
		if result.MatchedTop1 {
			top1Correct++
		}
		if result.MatchedTop3 {
			top3Correct++
		}
		latencies = append(latencies, result.LatencyMS)
	}

	report.Top1Accuracy = float64(top1Correct) / float64(len(results))
	report.Top3Accuracy = float64(top3Correct) / float64(len(results))
	report.AverageLatencyMS = averageLatency(latencies)
	report.P95LatencyMS = percentile95Latency(latencies)
	return report
}

func topCategoriesForFixture(fixture caseFixture, diagnostic *domain.FailureDiagnostic) []string {
	if strings.TrimSpace(fixture.Logs) == "" {
		if diagnostic == nil || strings.TrimSpace(diagnostic.FailureCategory) == "" {
			return nil
		}
		return []string{diagnostic.FailureCategory}
	}

	categories := make([]string, 0, 3)
	for _, candidate := range analysis.RankFailureDiagnoses(fixture.Logs, providerJobsFromGitHub(fixture.Jobs)) {
		categories = append(categories, candidate.FailureCategory)
		if len(categories) == 3 {
			break
		}
	}
	if len(categories) == 0 && diagnostic != nil && strings.TrimSpace(diagnostic.FailureCategory) != "" {
		return []string{diagnostic.FailureCategory}
	}
	return categories
}

func providerJobsFromGitHub(jobs []githubapi.Job) []providers.Job {
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
		})
	}
	return out
}

func containsCategory(categories []string, want string) bool {
	for _, category := range categories {
		if category == want {
			return true
		}
	}
	return false
}

func averageLatency(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func percentile95Latency(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := make([]float64, len(values))
	copy(copyValues, values)
	sort.Float64s(copyValues)
	idx := int(float64(len(copyValues)-1) * 0.95)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(copyValues) {
		idx = len(copyValues) - 1
	}
	return copyValues[idx]
}
