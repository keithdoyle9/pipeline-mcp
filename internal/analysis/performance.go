package analysis

import (
	"sort"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
)

func BuildPerformanceSnapshot(repository, workflow string, from, to time.Time, currentRuns, baselineRuns []githubapi.WorkflowRun) domain.PipelinePerformanceSnapshot {
	current := calculateWindowMetrics(currentRuns)
	baseline := calculateWindowMetrics(baselineRuns)

	return domain.PipelinePerformanceSnapshot{
		Repository: repository,
		Workflow:   workflow,
		From:       from.UTC().Format(time.RFC3339),
		To:         to.UTC().Format(time.RFC3339),
		Baseline:   baseline,
		Current:    current,
		Delta: domain.WindowDelta{
			SuccessRateChange:      current.SuccessRate - baseline.SuccessRate,
			MedianDurationMSChange: current.MedianDurationMS - baseline.MedianDurationMS,
			QueueTimeMSChange:      current.QueueTimeMS - baseline.QueueTimeMS,
			FlakyTestRateChange:    current.FlakyTestRate - baseline.FlakyTestRate,
		},
	}
}

func calculateWindowMetrics(runs []githubapi.WorkflowRun) domain.WindowMetrics {
	if len(runs) == 0 {
		return domain.WindowMetrics{FailureBreakdown: map[string]int{}}
	}

	durations := make([]int64, 0, len(runs))
	var successCount int
	var totalQueueTime int64
	failureBreakdown := map[string]int{}
	shaOutcomes := map[string]map[string]bool{}

	for _, run := range runs {
		durations = append(durations, runDurationMS(run))
		totalQueueTime += queueTimeMS(run)
		if strings.EqualFold(run.Conclusion, "success") {
			successCount++
		} else {
			failureBreakdown[classifyRunFailure(run)]++
		}

		if strings.TrimSpace(run.HeadSHA) != "" {
			if _, ok := shaOutcomes[run.HeadSHA]; !ok {
				shaOutcomes[run.HeadSHA] = map[string]bool{}
			}
			shaOutcomes[run.HeadSHA][strings.ToLower(run.Conclusion)] = true
		}
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	median := durations[len(durations)/2]
	if len(durations)%2 == 0 {
		median = (durations[len(durations)/2-1] + durations[len(durations)/2]) / 2
	}

	flakyCandidates := 0
	for _, outcomes := range shaOutcomes {
		if outcomes["failure"] && outcomes["success"] {
			flakyCandidates++
		}
	}

	total := len(runs)
	queueAvg := totalQueueTime / int64(total)

	return domain.WindowMetrics{
		SuccessRate:      float64(successCount) / float64(total),
		MedianDurationMS: median,
		QueueTimeMS:      queueAvg,
		FailureBreakdown: failureBreakdown,
		FlakyTestRate:    float64(flakyCandidates) / float64(total),
		TotalRuns:        total,
	}
}

func runDurationMS(run githubapi.WorkflowRun) int64 {
	end := run.UpdatedAt
	start := run.CreatedAt
	if run.RunStartedAt != nil && !run.RunStartedAt.IsZero() {
		start = *run.RunStartedAt
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func queueTimeMS(run githubapi.WorkflowRun) int64 {
	if run.RunStartedAt == nil || run.RunStartedAt.IsZero() {
		return 0
	}
	if run.RunStartedAt.Before(run.CreatedAt) {
		return 0
	}
	return run.RunStartedAt.Sub(run.CreatedAt).Milliseconds()
}

func classifyRunFailure(run githubapi.WorkflowRun) string {
	conclusion := strings.ToLower(strings.TrimSpace(run.Conclusion))
	switch conclusion {
	case "timed_out", "cancelled":
		return "infra_timeout"
	case "startup_failure", "action_required":
		return "config_error"
	case "failure":
		return "test_or_build_failure"
	case "stale":
		return "stale"
	default:
		if conclusion == "" {
			return "unknown"
		}
		return conclusion
	}
}
