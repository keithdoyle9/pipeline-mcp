package analysis

import (
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
)

func TestDiagnoseFailureTestCategory(t *testing.T) {
	logs := "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500"
	jobs := []githubapi.Job{{Name: "test", Conclusion: "failure"}}

	diagnostic, recommendations := DiagnoseFailure(logs, jobs)
	if diagnostic.FailureCategory != "test_failure" {
		t.Fatalf("expected test_failure, got %s", diagnostic.FailureCategory)
	}
	if len(diagnostic.EvidenceRefs) == 0 {
		t.Fatal("expected evidence refs")
	}
	if len(recommendations) == 0 {
		t.Fatal("expected recommendations")
	}
}

func TestDiagnoseFailureUnknownCategory(t *testing.T) {
	logs := "random log line without known patterns"
	diagnostic, recommendations := DiagnoseFailure(logs, nil)
	if diagnostic.FailureCategory != "unknown" {
		t.Fatalf("expected unknown, got %s", diagnostic.FailureCategory)
	}
	if len(recommendations) == 0 {
		t.Fatal("expected fallback recommendations")
	}
}
