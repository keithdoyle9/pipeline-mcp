package domain

import "time"

const (
	ProviderGitHub = "github_actions"
)

const (
	ErrorCodeUnauthorized        = "UNAUTHORIZED"
	ErrorCodeLogUnavailable      = "LOG_UNAVAILABLE"
	ErrorCodeRateLimited         = "RATE_LIMITED"
	ErrorCodeInvalidInput        = "INVALID_INPUT"
	ErrorCodeProviderUnavailable = "PROVIDER_UNAVAILABLE"
	ErrorCodeInternal            = "INTERNAL"
)

// ToolError is a shared structured error envelope for all tool responses.
type ToolError struct {
	Code        string         `json:"code"`
	Message     string         `json:"message"`
	Remediation string         `json:"remediation"`
	Retryable   bool           `json:"retryable"`
	Details     map[string]any `json:"details,omitempty"`
}

type PipelineRun struct {
	Provider      string `json:"provider"`
	Repository    string `json:"repository"`
	Workflow      string `json:"workflow"`
	Status        string `json:"status"`
	StartedAt     string `json:"started_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
	QueueTimeMS   int64  `json:"queue_time_ms"`
	RunURL        string `json:"run_url"`
	CommitSHA     string `json:"commit_sha"`
	RunID         int64  `json:"run_id"`
	Conclusion    string `json:"conclusion,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`
}

type EvidenceRef struct {
	Source  string `json:"source"`
	Line    int    `json:"line,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

type FailureDiagnostic struct {
	FailureCategory    string        `json:"failure_category"`
	SuspectedRootCause string        `json:"suspected_root_cause"`
	Confidence         float64       `json:"confidence"`
	EvidenceRefs       []EvidenceRef `json:"evidence_refs"`
	ImpactedJobs       []string      `json:"impacted_jobs"`
}

type FixRecommendation struct {
	RecommendationID string   `json:"recommendation_id"`
	Description      string   `json:"description"`
	ExpectedImpact   string   `json:"expected_impact"`
	Confidence       float64  `json:"confidence"`
	References       []string `json:"references,omitempty"`
}

type FlakyTest struct {
	TestName         string  `json:"test_name"`
	FailureFrequency int     `json:"failure_frequency"`
	FailureRate      float64 `json:"failure_rate"`
	Recency          string  `json:"recency"`
	Confidence       float64 `json:"confidence"`
}

type FlakyTestReport struct {
	Repository   string      `json:"repository"`
	Workflow     string      `json:"workflow,omitempty"`
	LookbackDays int         `json:"lookback_days"`
	ScannedRuns  int         `json:"scanned_runs"`
	FailedRuns   int         `json:"failed_runs"`
	TopFlaky     []FlakyTest `json:"top_flaky_tests"`
	GeneratedAt  string      `json:"generated_at"`
}

type WindowMetrics struct {
	SuccessRate      float64        `json:"success_rate"`
	MedianDurationMS int64          `json:"median_duration_ms"`
	QueueTimeMS      int64          `json:"queue_time_ms"`
	FailureBreakdown map[string]int `json:"failure_breakdown"`
	FlakyTestRate    float64        `json:"flaky_test_rate"`
	TotalRuns        int            `json:"total_runs"`
}

type WindowDelta struct {
	SuccessRateChange      float64 `json:"success_rate_change"`
	MedianDurationMSChange int64   `json:"median_duration_ms_change"`
	QueueTimeMSChange      int64   `json:"queue_time_ms_change"`
	FlakyTestRateChange    float64 `json:"flaky_test_rate_change"`
}

type PipelinePerformanceSnapshot struct {
	Repository string        `json:"repository"`
	Workflow   string        `json:"workflow"`
	From       string        `json:"from"`
	To         string        `json:"to"`
	Baseline   WindowMetrics `json:"baseline"`
	Current    WindowMetrics `json:"current"`
	Delta      WindowDelta   `json:"delta"`
}

type RerunResult struct {
	RunID       int64  `json:"run_id"`
	Repository  string `json:"repository"`
	Scope       string `json:"scope"`
	Status      string `json:"status"`
	Reason      string `json:"reason"`
	RequestedAt string `json:"requested_at"`
	Actor       string `json:"actor"`
}

type AuditEvent struct {
	EventID    string `json:"event_id"`
	Tool       string `json:"tool"`
	Actor      string `json:"actor"`
	Repository string `json:"repository"`
	RunID      int64  `json:"run_id"`
	Reason     string `json:"reason"`
	Scope      string `json:"scope"`
	Timestamp  string `json:"timestamp"`
	Outcome    string `json:"outcome"`
	Signature  string `json:"signature,omitempty"`
}

func NewToolError(code, message, remediation string, retryable bool, details map[string]any) *ToolError {
	return &ToolError{
		Code:        code,
		Message:     message,
		Remediation: remediation,
		Retryable:   retryable,
		Details:     details,
	}
}

func MustTimeString(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
