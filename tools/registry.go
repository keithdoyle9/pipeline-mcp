package tools

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/service"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Dependencies struct {
	Service   *service.Service
	Telemetry *telemetry.Collector
	Logger    *slog.Logger
}

type RunLocatorInput struct {
	RunURL     string `json:"run_url,omitempty" jsonschema:"GitHub Actions run URL"`
	RunID      int64  `json:"run_id,omitempty" jsonschema:"GitHub Actions run id"`
	Repository string `json:"repository,omitempty" jsonschema:"Repository in owner/repo format; required when using run_id, or accepted alone to resolve the latest failed run"`
}

type DiagnoseFailureInput struct {
	RunURL      string `json:"run_url,omitempty" jsonschema:"GitHub Actions run URL"`
	RunID       int64  `json:"run_id,omitempty" jsonschema:"GitHub Actions run id"`
	Repository  string `json:"repository,omitempty" jsonschema:"Repository in owner/repo format; required when using run_id, or accepted alone to diagnose the latest failed run"`
	MaxLogBytes int64  `json:"max_log_bytes,omitempty" jsonschema:"Max bytes of logs to ingest for analysis"`
}

type AnalyzeFlakyTestsInput struct {
	Repository   string `json:"repository" jsonschema:"Repository in owner/repo format"`
	LookbackDays int    `json:"lookback_days,omitempty" jsonschema:"How many days of run history to inspect"`
	Workflow     string `json:"workflow,omitempty" jsonschema:"Optional workflow name filter"`
}

type RerunInput struct {
	Repository     string `json:"repository" jsonschema:"Repository in owner/repo format"`
	RunID          int64  `json:"run_id" jsonschema:"GitHub Actions run id"`
	FailedJobsOnly bool   `json:"failed_jobs_only" jsonschema:"If true reruns only failed jobs"`
	Reason         string `json:"reason" jsonschema:"Reason for rerun to persist in audit log"`
}

type ComparePerformanceInput struct {
	Repository string `json:"repository" jsonschema:"Repository in owner/repo format"`
	Workflow   string `json:"workflow" jsonschema:"Workflow name to compare"`
	From       string `json:"from" jsonschema:"Current window start (RFC3339 or YYYY-MM-DD)"`
	To         string `json:"to" jsonschema:"Current window end (RFC3339 or YYYY-MM-DD)"`
}

type GetRunOutput struct {
	Run   *domain.PipelineRun `json:"run,omitempty"`
	Error *domain.ToolError   `json:"error,omitempty"`
}

type DiagnoseFailureOutput struct {
	Diagnostic      *domain.FailureDiagnostic  `json:"failure_diagnostic,omitempty"`
	Recommendations []domain.FixRecommendation `json:"fix_recommendations,omitempty"`
	Error           *domain.ToolError          `json:"error,omitempty"`
}

type AnalyzeFlakyTestsOutput struct {
	Report *domain.FlakyTestReport `json:"report,omitempty"`
	Error  *domain.ToolError       `json:"error,omitempty"`
}

type RerunOutput struct {
	Result *domain.RerunResult `json:"result,omitempty"`
	Error  *domain.ToolError   `json:"error,omitempty"`
}

type ComparePerformanceOutput struct {
	Snapshot *domain.PipelinePerformanceSnapshot `json:"snapshot,omitempty"`
	Error    *domain.ToolError                   `json:"error,omitempty"`
}

func Register(server *mcp.Server, deps Dependencies) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "pipeline.get_run",
		Description: "Fetch normalized metadata for a GitHub Actions workflow run by run_url, by run_id plus repository, or by repository alone for the latest failed run.",
	}, deps.getRun)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pipeline.diagnose_failure",
		Description: "Diagnose failed GitHub Actions runs and return ranked fix recommendations with evidence. Accepts run_url, run_id plus repository, or repository alone for the latest failed run.",
	}, deps.diagnoseFailure)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pipeline.analyze_flaky_tests",
		Description: "Analyze recent failed runs to identify likely flaky tests with confidence and recency.",
	}, deps.analyzeFlakyTests)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pipeline.rerun",
		Description: "Trigger controlled reruns for GitHub Actions with explicit reason and audit logging.",
	}, deps.rerun)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "pipeline.compare_performance",
		Description: "Compare workflow performance against the immediately preceding baseline window.",
	}, deps.comparePerformance)
}

func (d Dependencies) getRun(ctx context.Context, _ *mcp.CallToolRequest, input RunLocatorInput) (*mcp.CallToolResult, GetRunOutput, error) {
	start := time.Now()
	run, toolErr := d.Service.GetRun(ctx, service.RunReference{RunURL: input.RunURL, RunID: input.RunID, Repository: input.Repository})
	if toolErr != nil {
		d.observe("pipeline.get_run", start, false, nil)
		return toolErrorResult(toolErr), GetRunOutput{Error: toolErr}, nil
	}
	d.observe("pipeline.get_run", start, true, nil)
	return nil, GetRunOutput{Run: run}, nil
}

func (d Dependencies) diagnoseFailure(ctx context.Context, _ *mcp.CallToolRequest, input DiagnoseFailureInput) (*mcp.CallToolResult, DiagnoseFailureOutput, error) {
	start := time.Now()
	diagnostic, recommendations, toolErr := d.Service.DiagnoseFailure(ctx, service.RunReference{RunURL: input.RunURL, RunID: input.RunID, Repository: input.Repository}, input.MaxLogBytes)
	if toolErr != nil {
		d.observe("pipeline.diagnose_failure", start, false, nil)
		return toolErrorResult(toolErr), DiagnoseFailureOutput{Error: toolErr}, nil
	}
	confidence := diagnostic.Confidence
	d.observe("pipeline.diagnose_failure", start, true, &confidence)
	return nil, DiagnoseFailureOutput{Diagnostic: diagnostic, Recommendations: recommendations}, nil
}

func (d Dependencies) analyzeFlakyTests(ctx context.Context, _ *mcp.CallToolRequest, input AnalyzeFlakyTestsInput) (*mcp.CallToolResult, AnalyzeFlakyTestsOutput, error) {
	start := time.Now()
	report, toolErr := d.Service.AnalyzeFlakyTests(ctx, input.Repository, input.LookbackDays, input.Workflow)
	if toolErr != nil {
		d.observe("pipeline.analyze_flaky_tests", start, false, nil)
		return toolErrorResult(toolErr), AnalyzeFlakyTestsOutput{Error: toolErr}, nil
	}

	var confidence *float64
	if len(report.TopFlaky) > 0 {
		confidence = &report.TopFlaky[0].Confidence
	}
	d.observe("pipeline.analyze_flaky_tests", start, true, confidence)
	return nil, AnalyzeFlakyTestsOutput{Report: report}, nil
}

func (d Dependencies) rerun(ctx context.Context, _ *mcp.CallToolRequest, input RerunInput) (*mcp.CallToolResult, RerunOutput, error) {
	start := time.Now()
	result, toolErr := d.Service.Rerun(ctx, input.Repository, input.RunID, input.FailedJobsOnly, input.Reason)
	if toolErr != nil {
		d.observe("pipeline.rerun", start, false, nil)
		return toolErrorResult(toolErr), RerunOutput{Error: toolErr}, nil
	}
	d.observe("pipeline.rerun", start, true, nil)
	return nil, RerunOutput{Result: result}, nil
}

func (d Dependencies) comparePerformance(ctx context.Context, _ *mcp.CallToolRequest, input ComparePerformanceInput) (*mcp.CallToolResult, ComparePerformanceOutput, error) {
	start := time.Now()
	from, err := service.ParseDateTime(input.From)
	if err != nil {
		toolErr := domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), "Provide from as RFC3339 or YYYY-MM-DD.", false, nil)
		d.observe("pipeline.compare_performance", start, false, nil)
		return toolErrorResult(toolErr), ComparePerformanceOutput{Error: toolErr}, nil
	}
	to, err := service.ParseDateTime(input.To)
	if err != nil {
		toolErr := domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), "Provide to as RFC3339 or YYYY-MM-DD.", false, nil)
		d.observe("pipeline.compare_performance", start, false, nil)
		return toolErrorResult(toolErr), ComparePerformanceOutput{Error: toolErr}, nil
	}

	snapshot, toolErr := d.Service.ComparePerformance(ctx, input.Repository, input.Workflow, from, to)
	if toolErr != nil {
		d.observe("pipeline.compare_performance", start, false, nil)
		return toolErrorResult(toolErr), ComparePerformanceOutput{Error: toolErr}, nil
	}
	d.observe("pipeline.compare_performance", start, true, nil)
	return nil, ComparePerformanceOutput{Snapshot: snapshot}, nil
}

func (d Dependencies) observe(tool string, start time.Time, success bool, confidence *float64) {
	if d.Telemetry != nil {
		d.Telemetry.Observe(tool, time.Since(start), success, confidence)
	}
	if d.Logger != nil {
		d.Logger.Debug("tool invocation completed", "tool", tool, "success", success, "duration_ms", time.Since(start).Milliseconds())
	}
}

func toolErrorResult(toolErr *domain.ToolError) *mcp.CallToolResult {
	if toolErr == nil {
		return nil
	}
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: formatToolError(toolErr)}},
	}
}

func formatToolError(toolErr *domain.ToolError) string {
	if toolErr == nil {
		return ""
	}
	parts := []string{toolErr.Code + ": " + toolErr.Message}
	if strings.TrimSpace(toolErr.Remediation) != "" {
		parts = append(parts, "remediation: "+toolErr.Remediation)
	}
	return strings.Join(parts, " | ")
}
