package providers

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
)

type registryTestAdapter struct {
	providerID string
	runURL     string
}

func (a registryTestAdapter) ProviderID() string {
	return a.providerID
}

func (a registryTestAdapter) ParseRepository(repository string) (string, error) {
	return repository, nil
}

func (a registryTestAdapter) ParseRunURL(raw string) (*RunLocator, error) {
	if raw != a.runURL {
		return nil, errors.New("invalid run url")
	}
	return &RunLocator{Repository: a.providerID + "/repo", RunID: 42, RunURL: raw}, nil
}

func (a registryTestAdapter) ParseCheckRunURL(string) (int64, error) {
	return 0, nil
}

func (a registryTestAdapter) RunURL(repository string, runID int64) string {
	return a.runURL
}

func (a registryTestAdapter) GetRun(context.Context, string, int64) (*Run, error) {
	return nil, nil
}

func (a registryTestAdapter) ListRunJobs(context.Context, string, int64) ([]Job, error) {
	return nil, nil
}

func (a registryTestAdapter) DownloadRunLogs(context.Context, string, int64, int64) (string, error) {
	return "", nil
}

func (a registryTestAdapter) ListRepositoryRuns(context.Context, string, ListRunsOptions, int) ([]Run, error) {
	return nil, nil
}

func (a registryTestAdapter) GetCheckRun(context.Context, string, int64) (*CheckRun, error) {
	return nil, nil
}

func (a registryTestAdapter) GetCheckRunAnnotations(context.Context, string, int64) ([]CheckRunAnnotation, error) {
	return nil, nil
}

func (a registryTestAdapter) ListDeploymentBranchPolicies(context.Context, string, string) ([]BranchPolicy, error) {
	return nil, nil
}

func (a registryTestAdapter) Rerun(context.Context, string, int64, bool) error {
	return nil
}

func (a registryTestAdapter) IsLogsUnavailable(error) bool {
	return false
}

func (a registryTestAdapter) MapError(err error) *domain.ToolError {
	if err == nil {
		return nil
	}
	return domain.NewToolError(domain.ErrorCodeInternal, err.Error(), "retry", true, nil)
}

func TestRegistryResolveDefaultsToConfiguredProvider(t *testing.T) {
	registry, err := NewRegistry(
		"github_actions",
		registryTestAdapter{providerID: "github_actions", runURL: "https://github.example/runs/1"},
		registryTestAdapter{providerID: "gitlab_ci", runURL: "https://gitlab.example/pipelines/1"},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	adapter, err := registry.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if adapter.ProviderID() != "github_actions" {
		t.Fatalf("expected default provider github_actions, got %q", adapter.ProviderID())
	}
}

func TestRegistryResolveRunURLInfersProvider(t *testing.T) {
	registry, err := NewRegistry(
		"github_actions",
		registryTestAdapter{providerID: "github_actions", runURL: "https://github.example/runs/1"},
		registryTestAdapter{providerID: "gitlab_ci", runURL: "https://gitlab.example/pipelines/1"},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	adapter, locator, err := registry.ResolveRunURL("", "https://gitlab.example/pipelines/1")
	if err != nil {
		t.Fatalf("ResolveRunURL() error = %v", err)
	}
	if adapter.ProviderID() != "gitlab_ci" {
		t.Fatalf("expected gitlab_ci provider, got %q", adapter.ProviderID())
	}
	if locator == nil || locator.RunID != 42 {
		t.Fatalf("expected resolved locator, got %+v", locator)
	}
}

func TestRegistryResolveRejectsUnknownProvider(t *testing.T) {
	registry, err := NewRegistry(
		"github_actions",
		registryTestAdapter{providerID: "github_actions", runURL: "https://github.example/runs/1"},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}

	_, err = registry.Resolve("circleci")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported provider") {
		t.Fatalf("unexpected error: %v", err)
	}
}
