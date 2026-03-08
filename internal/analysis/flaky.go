package analysis

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
)

var testNamePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^--- FAIL:\s+([A-Za-z0-9_./-]+)`),
	regexp.MustCompile(`(?m)^FAIL\s+([A-Za-z0-9_./-]+)`),
	regexp.MustCompile(`(?m)test\s+"([^"]+)"\s+failed`),
	regexp.MustCompile(`(?m)^\s*[x✗]\s+([A-Za-z0-9_./:-]+)`),
}

type LogFetcher func(runID int64) (string, error)

type flakyAggregate struct {
	count      int
	lastSeenAt time.Time
}

func AnalyzeFlakyTests(
	repository, workflow string,
	lookbackDays int,
	runs []providers.Run,
	fetchLogs LogFetcher,
	now time.Time,
) domain.FlakyTestReport {
	aggregates := map[string]*flakyAggregate{}
	failedRuns := 0

	for _, run := range runs {
		if !strings.EqualFold(run.Conclusion, "failure") && !strings.EqualFold(run.Status, "failure") {
			continue
		}
		failedRuns++
		if fetchLogs == nil {
			continue
		}
		logs, err := fetchLogs(run.ID)
		if err != nil {
			continue
		}
		seenInRun := map[string]struct{}{}
		for _, name := range extractTestNames(logs) {
			if _, exists := seenInRun[name]; exists {
				continue
			}
			seenInRun[name] = struct{}{}
			entry, ok := aggregates[name]
			if !ok {
				entry = &flakyAggregate{}
				aggregates[name] = entry
			}
			entry.count++
			if run.UpdatedAt.After(entry.lastSeenAt) {
				entry.lastSeenAt = run.UpdatedAt
			}
		}
	}

	records := make([]domain.FlakyTest, 0, len(aggregates))
	denominator := failedRuns
	if denominator == 0 {
		denominator = 1
	}
	for testName, item := range aggregates {
		rate := float64(item.count) / float64(denominator)
		recency := "unknown"
		if !item.lastSeenAt.IsZero() {
			recency = item.lastSeenAt.UTC().Format(time.RFC3339)
		}
		confidence := 0.3 + 0.4*rate + 0.08*float64(item.count)
		if confidence > 0.98 {
			confidence = 0.98
		}
		records = append(records, domain.FlakyTest{
			TestName:         testName,
			FailureFrequency: item.count,
			FailureRate:      rate,
			Recency:          recency,
			Confidence:       confidence,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].FailureFrequency == records[j].FailureFrequency {
			return records[i].Confidence > records[j].Confidence
		}
		return records[i].FailureFrequency > records[j].FailureFrequency
	})
	if len(records) > 10 {
		records = records[:10]
	}

	return domain.FlakyTestReport{
		Repository:   repository,
		Workflow:     workflow,
		LookbackDays: lookbackDays,
		ScannedRuns:  len(runs),
		FailedRuns:   failedRuns,
		TopFlaky:     records,
		GeneratedAt:  now.UTC().Format(time.RFC3339),
	}
}

func extractTestNames(logs string) []string {
	logs = RedactSecrets(logs)
	out := make([]string, 0, 8)
	for _, pattern := range testNamePatterns {
		matches := pattern.FindAllStringSubmatch(logs, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			name := strings.TrimSpace(match[1])
			if name == "" {
				continue
			}
			out = append(out, name)
		}
	}
	return dedupeStrings(out)
}
