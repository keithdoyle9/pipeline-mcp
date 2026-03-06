package main

import (
	"testing"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
)

func TestTopCategoriesForFixtureUsesRankedDiagnoses(t *testing.T) {
	fixture := caseFixture{
		Logs: "--- FAIL: TestCheckout\n" +
			"AssertionError: expected 200 got 500\n" +
			"npm ERR! network request failed\n" +
			"503 service unavailable\n" +
			"context deadline exceeded\n" +
			"step timed out",
	}

	got := topCategoriesForFixture(
		fixture,
		[]githubapi.Job{{Name: "test", Conclusion: "failure"}},
		&domain.FailureDiagnostic{FailureCategory: "test_failure"},
	)

	want := []string{"test_failure", "dependency_outage", "infra_timeout"}
	if len(got) != len(want) {
		t.Fatalf("expected %d categories, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("category %d = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestTopCategoriesForFixtureFallsBackToServiceCategory(t *testing.T) {
	got := topCategoriesForFixture(
		caseFixture{},
		[]githubapi.Job{{Name: "deploy", Conclusion: "failure"}},
		&domain.FailureDiagnostic{FailureCategory: "config_error"},
	)

	if len(got) != 1 || got[0] != "config_error" {
		t.Fatalf("unexpected categories: %v", got)
	}
}

func TestSummarizeResultsComputesAccuracyAndLatency(t *testing.T) {
	report := summarizeResults([]benchmarkCaseResult{
		{MatchedTop1: true, MatchedTop3: true, LatencyMS: 10},
		{MatchedTop1: false, MatchedTop3: true, LatencyMS: 20},
		{MatchedTop1: false, MatchedTop3: false, LatencyMS: 30},
	})

	if report.Cases != 3 {
		t.Fatalf("expected 3 cases, got %d", report.Cases)
	}
	if report.Top1Accuracy != 1.0/3.0 {
		t.Fatalf("unexpected top1 accuracy: %f", report.Top1Accuracy)
	}
	if report.Top3Accuracy != 2.0/3.0 {
		t.Fatalf("unexpected top3 accuracy: %f", report.Top3Accuracy)
	}
	if report.AverageLatencyMS != 20 {
		t.Fatalf("expected average latency 20, got %f", report.AverageLatencyMS)
	}
	if report.P95LatencyMS != 20 {
		t.Fatalf("expected p95 latency 20, got %f", report.P95LatencyMS)
	}
}
