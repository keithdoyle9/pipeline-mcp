package tools

import (
	"context"
	"io"
	"log/slog"
	"sort"
	"testing"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/audit"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
	"github.com/keithdoyle9/pipeline-mcp/internal/service"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type noopGitHubClient struct{}

func (noopGitHubClient) GetRun(context.Context, string, string, int64) (*githubapi.WorkflowRun, error) {
	return nil, nil
}
func (noopGitHubClient) ListRunJobs(context.Context, string, string, int64) ([]githubapi.Job, error) {
	return nil, nil
}
func (noopGitHubClient) DownloadRunLogs(context.Context, string, string, int64, int64) (string, error) {
	return "", nil
}
func (noopGitHubClient) ListRepositoryRuns(context.Context, string, string, githubapi.ListRunsOptions, int) ([]githubapi.WorkflowRun, error) {
	return nil, nil
}
func (noopGitHubClient) GetCheckRun(context.Context, string, string, int64) (*githubapi.CheckRun, error) {
	return nil, nil
}
func (noopGitHubClient) GetCheckRunAnnotations(context.Context, string, string, int64) ([]githubapi.CheckRunAnnotation, error) {
	return nil, nil
}
func (noopGitHubClient) ListDeploymentBranchPolicies(context.Context, string, string, string) ([]githubapi.BranchPolicy, error) {
	return nil, nil
}
func (noopGitHubClient) Rerun(context.Context, string, string, int64, bool) error {
	return nil
}

func TestRegisterExposesFiveTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "pipeline-mcp-test", Version: "test"}, &mcp.ServerOptions{Capabilities: &mcp.ServerCapabilities{Tools: &mcp.ToolCapabilities{}}})
	svc := service.New(
		&config.Config{GitHubAPIBaseURL: "https://api.github.com", DisableMutations: true, MaxLogBytes: 1024, DefaultLookbackDays: 14, MaxHistoricalRuns: 100},
		githubapi.NewProviderAdapter(noopGitHubClient{}, "https://api.github.com"),
		audit.NewJSONLStore(t.TempDir()+"/audit.jsonl", ""),
		telemetry.NewCollector(""),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	Register(server, Dependencies{Service: svc, Telemetry: telemetry.NewCollector(""), Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, serverTransport)
	}()

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)

	want := []string{
		"pipeline.analyze_flaky_tests",
		"pipeline.compare_performance",
		"pipeline.diagnose_failure",
		"pipeline.get_run",
		"pipeline.rerun",
	}
	if len(names) != len(want) {
		t.Fatalf("expected %d tools, got %d (%v)", len(want), len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unexpected tools: got %v want %v", names, want)
		}
	}

	cancel()
	select {
	case <-time.After(500 * time.Millisecond):
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			t.Fatalf("server run failed: %v", err)
		}
	}
}
