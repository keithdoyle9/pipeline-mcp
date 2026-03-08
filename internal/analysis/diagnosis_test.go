package analysis

import (
	"strings"
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

func TestDiagnoseFailureTestCategory(t *testing.T) {
	logs := "--- FAIL: TestCheckout\nAssertionError: expected 200 got 500"
	jobs := []providers.Job{{Name: "test", Conclusion: "failure"}}

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

func TestRankFailureDiagnosesOrdersCategoriesByScore(t *testing.T) {
	logs := strings.Join([]string{
		"--- FAIL: TestCheckout",
		"AssertionError: expected 200 got 500",
		"npm ERR! network request failed",
		"503 service unavailable",
		"context deadline exceeded",
		"step timed out",
	}, "\n")

	ranked := RankFailureDiagnoses(logs, []providers.Job{{Name: "test", Conclusion: "failure"}})
	if len(ranked) < 3 {
		t.Fatalf("expected at least 3 ranked diagnoses, got %d", len(ranked))
	}

	want := []string{"test_failure", "dependency_outage", "infra_timeout"}
	for i := range want {
		if ranked[i].FailureCategory != want[i] {
			t.Fatalf("rank %d = %s, want %s", i, ranked[i].FailureCategory, want[i])
		}
	}
}
