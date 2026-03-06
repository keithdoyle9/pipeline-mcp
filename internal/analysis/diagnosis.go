package analysis

import (
	"regexp"
	"sort"
	"strings"

	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/githubapi"
)

type categoryRule struct {
	category        string
	rootCause       string
	recommendations []domain.FixRecommendation
	patterns        []*regexp.Regexp
}

type RankedDiagnosis struct {
	FailureCategory    string
	SuspectedRootCause string
	Confidence         float64
	Score              int
	EvidenceRefs       []domain.EvidenceRef
	Recommendations    []domain.FixRecommendation
}

var diagnosisRules = []categoryRule{
	{
		category:  "test_failure",
		rootCause: "One or more tests failed under current repository state.",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)^--- FAIL:\s+\S+`),
			regexp.MustCompile(`(?m)\bAssertionError\b`),
			regexp.MustCompile(`(?m)\bexpected\b.*\bgot\b`),
			regexp.MustCompile(`(?m)\b(\d+)\s+tests?\s+failed\b`),
		},
		recommendations: []domain.FixRecommendation{
			{RecommendationID: "re-run-failed-tests-locally", Description: "Reproduce the failing test subset locally with the same test flags.", ExpectedImpact: "Validates deterministic failure and narrows scope.", Confidence: 0.84, References: []string{"https://docs.github.com/actions"}},
			{RecommendationID: "stabilize-test-fixtures", Description: "Harden fixtures/mocks and isolate shared mutable test state.", ExpectedImpact: "Reduces flaky and order-dependent failures.", Confidence: 0.72},
		},
	},
	{
		category:  "dependency_outage",
		rootCause: "External dependency or package source appears unavailable.",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)could not resolve host`),
			regexp.MustCompile(`(?m)connection refused`),
			regexp.MustCompile(`(?m)503\s+service\s+unavailable`),
			regexp.MustCompile(`(?m)429\s+too\s+many\s+requests`),
			regexp.MustCompile(`(?m)npm\s+ERR!\s+network`),
		},
		recommendations: []domain.FixRecommendation{
			{RecommendationID: "retry-with-backoff", Description: "Retry pipeline with exponential backoff for transient external dependency errors.", ExpectedImpact: "Improves resilience to temporary outages.", Confidence: 0.8},
			{RecommendationID: "cache-dependencies", Description: "Enable or tighten dependency caching in workflow jobs.", ExpectedImpact: "Reduces dependence on upstream availability.", Confidence: 0.74},
		},
	},
	{
		category:  "infra_timeout",
		rootCause: "Runner, network, or step execution timed out.",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)timed out`),
			regexp.MustCompile(`(?m)context deadline exceeded`),
			regexp.MustCompile(`(?m)runner.+lost communication`),
			regexp.MustCompile(`(?m)no space left on device`),
		},
		recommendations: []domain.FixRecommendation{
			{RecommendationID: "split-long-running-jobs", Description: "Split long workflow stages into smaller jobs and parallelize where possible.", ExpectedImpact: "Lowers timeout risk and improves observability.", Confidence: 0.76},
			{RecommendationID: "increase-timeouts-selectively", Description: "Increase timeout for affected jobs only after verifying workload growth.", ExpectedImpact: "Avoids false failures while limiting regression risk.", Confidence: 0.68},
		},
	},
	{
		category:  "config_error",
		rootCause: "Workflow or environment configuration is invalid.",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)workflow\s+is\s+not\s+valid`),
			regexp.MustCompile(`(?m)yaml:\s+line`),
			regexp.MustCompile(`(?m)permission denied`),
			regexp.MustCompile(`(?m)no such file or directory`),
		},
		recommendations: []domain.FixRecommendation{
			{RecommendationID: "validate-workflow-config", Description: "Validate workflow YAML and referenced paths/secrets in the failing job.", ExpectedImpact: "Fixes deterministic configuration failures quickly.", Confidence: 0.81},
			{RecommendationID: "check-token-scopes", Description: "Confirm token scopes and permissions required by the failed step.", ExpectedImpact: "Resolves authorization/config mismatches.", Confidence: 0.73},
		},
	},
	{
		category:  "compilation_error",
		rootCause: "Build or compilation failed due to code errors.",
		patterns: []*regexp.Regexp{
			regexp.MustCompile(`(?m)syntax error`),
			regexp.MustCompile(`(?m)undefined:\s+\S+`),
			regexp.MustCompile(`(?m)cannot find package`),
			regexp.MustCompile(`(?m)build failed`),
		},
		recommendations: []domain.FixRecommendation{
			{RecommendationID: "fix-build-errors", Description: "Address compile/type errors shown in failed build output.", ExpectedImpact: "Restores buildability and unblocks downstream tests.", Confidence: 0.9},
			{RecommendationID: "pin-toolchain", Description: "Pin compiler/runtime versions in CI to avoid environment drift.", ExpectedImpact: "Prevents toolchain-induced regressions.", Confidence: 0.65},
		},
	},
}

func RankFailureDiagnoses(logs string, jobs []githubapi.Job) []RankedDiagnosis {
	ranked, _ := rankFailureDiagnoses(logs, jobs)
	return ranked
}

func rankFailureDiagnoses(logs string, jobs []githubapi.Job) ([]RankedDiagnosis, []string) {
	logs = RedactSecrets(logs)
	impactedJobs := failedJobs(jobs)

	ranked := make([]RankedDiagnosis, 0, len(diagnosisRules))

	for _, rule := range diagnosisRules {
		score := 0
		matched := make([]domain.EvidenceRef, 0, 3)
		for _, pattern := range rule.patterns {
			found := pattern.FindAllStringIndex(logs, -1)
			score += len(found)
			if len(found) > 0 && len(matched) < 3 {
				matched = append(matched, extractEvidence(logs, found, 3-len(matched))...)
			}
		}
		if score == 0 {
			continue
		}

		recommendations := cloneRecommendations(rule.recommendations)
		sort.SliceStable(recommendations, func(i, j int) bool {
			return recommendations[i].Confidence > recommendations[j].Confidence
		})

		ranked = append(ranked, RankedDiagnosis{
			FailureCategory:    rule.category,
			SuspectedRootCause: rule.rootCause,
			Confidence:         scoreToConfidence(score, len(matched), len(impactedJobs), rule.category),
			Score:              score,
			EvidenceRefs:       matched,
			Recommendations:    recommendations,
		})
	}

	if len(ranked) == 0 {
		return []RankedDiagnosis{unknownRankedDiagnosis(logs, impactedJobs)}, impactedJobs
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Score == ranked[j].Score {
			if len(ranked[i].EvidenceRefs) == len(ranked[j].EvidenceRefs) {
				return ranked[i].Confidence > ranked[j].Confidence
			}
			return len(ranked[i].EvidenceRefs) > len(ranked[j].EvidenceRefs)
		}
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, impactedJobs
}

func DiagnoseFailure(logs string, jobs []githubapi.Job) (domain.FailureDiagnostic, []domain.FixRecommendation) {
	ranked, impactedJobs := rankFailureDiagnoses(logs, jobs)
	if len(ranked) == 0 {
		ranked = []RankedDiagnosis{unknownRankedDiagnosis(RedactSecrets(logs), impactedJobs)}
	}
	best := ranked[0]

	return domain.FailureDiagnostic{
		FailureCategory:    best.FailureCategory,
		SuspectedRootCause: best.SuspectedRootCause,
		Confidence:         best.Confidence,
		EvidenceRefs:       best.EvidenceRefs,
		ImpactedJobs:       impactedJobs,
	}, best.Recommendations
}

func failedJobs(jobs []githubapi.Job) []string {
	out := make([]string, 0, len(jobs))
	for _, job := range jobs {
		if strings.EqualFold(job.Conclusion, "failure") || strings.EqualFold(job.Status, "failed") {
			out = append(out, job.Name)
		}
	}
	if len(out) == 0 {
		for _, job := range jobs {
			if !strings.EqualFold(job.Conclusion, "success") {
				out = append(out, job.Name)
			}
		}
	}
	return dedupeStrings(out)
}

func extractEvidence(logs string, matches [][]int, limit int) []domain.EvidenceRef {
	if limit <= 0 || len(matches) == 0 {
		return nil
	}
	lines := strings.Split(logs, "\n")
	out := make([]domain.EvidenceRef, 0, limit)
	pos := 0
	for idx, line := range lines {
		start := pos
		end := start + len(line)
		pos = end + 1
		for _, match := range matches {
			if match[0] >= start && match[0] <= end {
				out = append(out, domain.EvidenceRef{Source: "run_logs", Line: idx + 1, Snippet: strings.TrimSpace(line)})
				break
			}
		}
		if len(out) >= limit {
			break
		}
	}
	return out
}

func fallbackEvidence(logs string, impactedJobs []string) []domain.EvidenceRef {
	evidence := make([]domain.EvidenceRef, 0, 2)
	if len(impactedJobs) > 0 {
		evidence = append(evidence, domain.EvidenceRef{Source: "job_summary", Snippet: "Failed jobs: " + strings.Join(impactedJobs, ", ")})
	}
	trimmed := strings.TrimSpace(logs)
	if trimmed != "" {
		first := trimmed
		if len(first) > 220 {
			first = first[:220]
		}
		evidence = append(evidence, domain.EvidenceRef{Source: "run_logs", Line: 1, Snippet: first})
	}
	return evidence
}

func scoreToConfidence(score int, evidenceCount int, impactedJobs int, category string) float64 {
	if category == "unknown" {
		if evidenceCount > 0 {
			return 0.4
		}
		return 0.25
	}
	confidence := 0.45 + 0.08*float64(score) + 0.04*float64(evidenceCount)
	if impactedJobs > 0 {
		confidence += 0.03
	}
	if confidence > 0.97 {
		confidence = 0.97
	}
	return confidence
}

func cloneRecommendations(in []domain.FixRecommendation) []domain.FixRecommendation {
	out := make([]domain.FixRecommendation, len(in))
	copy(out, in)
	return out
}

func unknownRecommendations() []domain.FixRecommendation {
	return []domain.FixRecommendation{
		{RecommendationID: "rerun-failed-jobs-only", Description: "Rerun failed jobs to distinguish transient failure from deterministic breakage.", ExpectedImpact: "Quickly detects transient infrastructure issues.", Confidence: 0.58},
		{RecommendationID: "collect-more-context", Description: "Retrieve full logs and inspect first failing step in each impacted job.", ExpectedImpact: "Improves diagnosis confidence with richer evidence.", Confidence: 0.55},
	}
}

func unknownRankedDiagnosis(logs string, impactedJobs []string) RankedDiagnosis {
	evidence := fallbackEvidence(logs, impactedJobs)
	return RankedDiagnosis{
		FailureCategory:    "unknown",
		SuspectedRootCause: "Unable to infer root cause from available evidence.",
		Confidence:         scoreToConfidence(0, len(evidence), len(impactedJobs), "unknown"),
		Score:              0,
		EvidenceRefs:       evidence,
		Recommendations:    unknownRecommendations(),
	}
}

func dedupeStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, item := range in {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
