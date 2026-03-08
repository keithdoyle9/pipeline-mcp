package analysis

import (
	"fmt"
	"testing"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

func TestAnalyzeFlakyTests(t *testing.T) {
	now := time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC)
	runs := []providers.Run{
		{ID: 1, Conclusion: "failure", UpdatedAt: now.Add(-2 * time.Hour)},
		{ID: 2, Conclusion: "failure", UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: 3, Conclusion: "success", UpdatedAt: now.Add(-30 * time.Minute)},
	}

	logsByRun := map[int64]string{
		1: "--- FAIL: TestCheckout\n",
		2: "--- FAIL: TestCheckout\n--- FAIL: TestBilling\n",
	}

	report := AnalyzeFlakyTests("acme/repo", "", 14, runs, func(runID int64) (string, error) {
		v, ok := logsByRun[runID]
		if !ok {
			return "", fmt.Errorf("missing logs")
		}
		return v, nil
	}, now)

	if report.FailedRuns != 2 {
		t.Fatalf("expected 2 failed runs, got %d", report.FailedRuns)
	}
	if len(report.TopFlaky) == 0 {
		t.Fatal("expected flaky tests")
	}
	if report.TopFlaky[0].TestName != "TestCheckout" {
		t.Fatalf("expected TestCheckout first, got %s", report.TopFlaky[0].TestName)
	}
}
