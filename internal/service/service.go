package service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/keithdoyle9/pipeline-mcp/config"
	"github.com/keithdoyle9/pipeline-mcp/internal/analysis"
	"github.com/keithdoyle9/pipeline-mcp/internal/audit"
	"github.com/keithdoyle9/pipeline-mcp/internal/domain"
	"github.com/keithdoyle9/pipeline-mcp/internal/providers"
	"github.com/keithdoyle9/pipeline-mcp/internal/telemetry"
)

type Service struct {
	cfg       *config.Config
	providers *providers.Registry
	audit     audit.Store
	telemetry *telemetry.Collector
	logger    *slog.Logger
	now       func() time.Time
}

func New(cfg *config.Config, registry *providers.Registry, auditStore audit.Store, collector *telemetry.Collector, logger *slog.Logger) *Service {
	return &Service{
		cfg:       cfg,
		providers: registry,
		audit:     auditStore,
		telemetry: collector,
		logger:    logger,
		now:       time.Now,
	}
}

type RunReference struct {
	Provider   string
	RunURL     string
	RunID      int64
	Repository string
}

func (s *Service) GetRun(ctx context.Context, input RunReference) (*domain.PipelineRun, *domain.ToolError) {
	provider, resolved, toolErr := s.resolveRunReference(ctx, input, true)
	if toolErr != nil {
		return nil, toolErr
	}

	run, err := provider.GetRun(ctx, resolved.Repository, resolved.RunID)
	if err != nil {
		return nil, provider.MapError(err)
	}

	jobs, err := provider.ListRunJobs(ctx, resolved.Repository, resolved.RunID)
	if err != nil {
		s.logger.Warn("failed to fetch jobs for run", "repository", resolved.Repository, "run_id", resolved.RunID, "error", err)
	}

	pipelineRun := normalizeRun(provider.ProviderID(), run, resolved.Repository, resolved.RunURL, jobs)
	return &pipelineRun, nil
}

func (s *Service) DiagnoseFailure(ctx context.Context, input RunReference, maxLogBytes int64) (*domain.FailureDiagnostic, []domain.FixRecommendation, *domain.ToolError) {
	provider, resolved, toolErr := s.resolveRunReference(ctx, input, true)
	if toolErr != nil {
		return nil, nil, toolErr
	}

	if maxLogBytes <= 0 {
		maxLogBytes = s.cfg.MaxLogBytes
	}
	if maxLogBytes > s.cfg.MaxLogBytes {
		maxLogBytes = s.cfg.MaxLogBytes
	}

	run, err := provider.GetRun(ctx, resolved.Repository, resolved.RunID)
	if err != nil {
		return nil, nil, provider.MapError(err)
	}
	jobs, err := provider.ListRunJobs(ctx, resolved.Repository, resolved.RunID)
	if err != nil {
		return nil, nil, provider.MapError(err)
	}
	if diagnostic, recommendations, ok := s.diagnoseMetadataFailure(ctx, provider, resolved.Repository, jobs); ok {
		return diagnostic, recommendations, nil
	}

	logs, err := provider.DownloadRunLogs(ctx, resolved.Repository, resolved.RunID, maxLogBytes)
	if err != nil {
		if provider.IsLogsUnavailable(err) {
			return nil, nil, domain.NewToolError(
				domain.ErrorCodeLogUnavailable,
				"Workflow logs are unavailable for this run.",
				"Confirm the run still exists and your token has actions:read access, then retry.",
				true,
				map[string]any{"repository": resolved.Repository, "run_id": resolved.RunID},
			)
		}
		return nil, nil, provider.MapError(err)
	}

	redacted := analysis.RedactSecrets(logs)
	diagnostic, recommendations := analysis.DiagnoseFailure(redacted, jobs)
	if diagnostic.SuspectedRootCause == "" && run != nil {
		diagnostic.SuspectedRootCause = "Run failed but logs did not include a clear signature."
	}
	return &diagnostic, recommendations, nil
}

var envProtectionPattern = regexp.MustCompile(`Branch "([^"]+)" is not allowed to deploy to ([^ ]+) due to environment protection rules\.`)
var approvalRequiredPattern = regexp.MustCompile(`(?i)(review required|required reviewers|approved review|approval.+required|not approved by required reviewers|awaiting review)`)

func (s *Service) diagnoseMetadataFailure(ctx context.Context, provider providers.Adapter, repository string, jobs []providers.Job) (*domain.FailureDiagnostic, []domain.FixRecommendation, bool) {
	for _, job := range jobs {
		if !strings.EqualFold(job.Conclusion, "failure") {
			continue
		}
		if len(job.Steps) > 0 || strings.TrimSpace(job.CheckRunURL) == "" {
			continue
		}

		checkRunID, err := provider.ParseCheckRunURL(job.CheckRunURL)
		if err != nil {
			continue
		}
		annotations, err := provider.GetCheckRunAnnotations(ctx, repository, checkRunID)
		if err != nil || len(annotations) == 0 {
			continue
		}

		var branch string
		environment := ""
		evidence := make([]domain.EvidenceRef, 0, len(annotations))
		for _, annotation := range annotations {
			message := strings.TrimSpace(annotation.Message)
			if message == "" {
				continue
			}
			evidence = append(evidence, domain.EvidenceRef{
				Source:  "check_run_annotation",
				Line:    annotation.StartLine,
				Snippet: message,
			})
			if matches := envProtectionPattern.FindStringSubmatch(message); len(matches) == 3 {
				branch = matches[1]
				environment = matches[2]
			}
		}
		if len(evidence) == 0 {
			continue
		}

		checkRun, err := provider.GetCheckRun(ctx, repository, checkRunID)
		if err == nil && checkRun != nil && checkRun.Deployment != nil {
			if environment == "" {
				environment = firstNonEmpty(checkRun.Deployment.OriginalEnvironment, checkRun.Deployment.Environment)
			}
		}
		if branch == "" {
			branch = job.HeadBranch
		}
		if environment == "" {
			environment = "the target environment"
		}

		if !containsEnvProtectionEvidence(evidence) {
			if containsApprovalGateEvidence(evidence) {
				recommendations := buildApprovalGateRecommendations(environment)
				diagnostic := &domain.FailureDiagnostic{
					FailureCategory:    "config_error",
					SuspectedRootCause: fmt.Sprintf("Deployment to %s is blocked pending required reviewer approval.", environment),
					Confidence:         0.92,
					EvidenceRefs:       evidence,
					ImpactedJobs:       []string{job.Name},
				}
				return diagnostic, recommendations, true
			}
			continue
		}

		var policies []providers.BranchPolicy
		if environment != "the target environment" {
			policies, err = provider.ListDeploymentBranchPolicies(ctx, repository, environment)
			if err != nil {
				policies = nil
			}
		}
		policyNames := make([]string, 0, len(policies))
		for _, policy := range policies {
			if strings.TrimSpace(policy.Name) != "" {
				policyNames = append(policyNames, policy.Name)
			}
		}

		rootCause := fmt.Sprintf("Branch %q is blocked by environment protection rules for %s.", branch, environment)
		if len(policyNames) > 0 {
			rootCause = fmt.Sprintf("%s Allowed deployment branches currently match: %s.", rootCause, strings.Join(policyNames, ", "))
		}

		recommendations := buildEnvProtectionRecommendations(branch, environment, policyNames)
		diagnostic := &domain.FailureDiagnostic{
			FailureCategory:    "config_error",
			SuspectedRootCause: rootCause,
			Confidence:         0.94,
			EvidenceRefs:       evidence,
			ImpactedJobs:       []string{job.Name},
		}
		return diagnostic, recommendations, true
	}

	return nil, nil, false
}

func (s *Service) AnalyzeFlakyTests(ctx context.Context, providerID, repository string, lookbackDays int, workflow string) (*domain.FlakyTestReport, *domain.ToolError) {
	provider, toolErr := s.resolveProvider(providerID)
	if toolErr != nil {
		return nil, toolErr
	}

	repository, err := provider.ParseRepository(repository)
	if err != nil {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), s.repositoryRemediation(provider.ProviderID()), false, providerDetails(provider.ProviderID(), s.providers.ProviderIDs()))
	}
	if lookbackDays <= 0 {
		lookbackDays = s.cfg.DefaultLookbackDays
	}
	if lookbackDays > 90 {
		lookbackDays = 90
	}

	end := s.now().UTC()
	start := end.AddDate(0, 0, -lookbackDays)
	created := fmt.Sprintf("%s..%s", start.Format(time.RFC3339), end.Format(time.RFC3339))
	runs, err := provider.ListRepositoryRuns(ctx, repository, providers.ListRunsOptions{Created: created}, s.cfg.MaxHistoricalRuns)
	if err != nil {
		return nil, provider.MapError(err)
	}
	runs = filterByWorkflow(runs, workflow)

	report := analysis.AnalyzeFlakyTests(repository, workflow, lookbackDays, runs, func(runID int64) (string, error) {
		return provider.DownloadRunLogs(ctx, repository, runID, s.cfg.MaxLogBytes/2)
	}, end)
	return &report, nil
}

func (s *Service) Rerun(ctx context.Context, providerID, repository string, runID int64, failedJobsOnly bool, reason string) (*domain.RerunResult, *domain.ToolError) {
	provider, toolErr := s.resolveProvider(providerID)
	if toolErr != nil {
		return nil, toolErr
	}

	if strings.TrimSpace(reason) == "" {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, "reason is required", "Provide a short reason for auditability.", false, nil)
	}
	repository, err := provider.ParseRepository(repository)
	if err != nil {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), s.repositoryRemediation(provider.ProviderID()), false, providerDetails(provider.ProviderID(), s.providers.ProviderIDs()))
	}
	if runID <= 0 {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, "run_id must be greater than zero", fmt.Sprintf("Pass a valid run id for provider %q.", provider.ProviderID()), false, nil)
	}
	if s.cfg.DisableMutations {
		return nil, domain.NewToolError(
			domain.ErrorCodeUnauthorized,
			"Mutation tools are disabled by configuration.",
			"Set DISABLE_MUTATIONS=false and provide GITHUB_WRITE_TOKEN with actions:write.",
			false,
			nil,
		)
	}

	scope := "full_run"
	if failedJobsOnly {
		scope = "failed_jobs_only"
	}
	result := &domain.RerunResult{
		RunID:       runID,
		Repository:  repository,
		Scope:       scope,
		Status:      "requested",
		Reason:      reason,
		RequestedAt: s.now().UTC().Format(time.RFC3339),
		Actor:       s.cfg.Actor,
	}

	err = provider.Rerun(ctx, repository, runID, failedJobsOnly)
	outcome := "success"
	if err != nil {
		outcome = "failed"
		mapped := provider.MapError(err)
		_ = s.emitAudit(ctx, repository, runID, reason, scope, outcome)
		return nil, mapped
	}

	if auditErr := s.emitAudit(ctx, repository, runID, reason, scope, outcome); auditErr != nil {
		s.logger.Error("failed to persist audit event", "error", auditErr)
	}

	return result, nil
}

func (s *Service) ComparePerformance(ctx context.Context, providerID, repository, workflow string, from, to time.Time) (*domain.PipelinePerformanceSnapshot, *domain.ToolError) {
	provider, toolErr := s.resolveProvider(providerID)
	if toolErr != nil {
		return nil, toolErr
	}

	repository, err := provider.ParseRepository(repository)
	if err != nil {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), s.repositoryRemediation(provider.ProviderID()), false, providerDetails(provider.ProviderID(), s.providers.ProviderIDs()))
	}
	if to.Before(from) || to.Equal(from) {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, "to must be after from", "Provide a time range where to > from.", false, nil)
	}

	duration := to.Sub(from)
	baselineFrom := from.Add(-duration)
	baselineTo := from

	currentRange := fmt.Sprintf("%s..%s", from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	baselineRange := fmt.Sprintf("%s..%s", baselineFrom.UTC().Format(time.RFC3339), baselineTo.UTC().Format(time.RFC3339))

	currentRuns, err := provider.ListRepositoryRuns(ctx, repository, providers.ListRunsOptions{Created: currentRange}, s.cfg.MaxHistoricalRuns)
	if err != nil {
		return nil, provider.MapError(err)
	}
	baselineRuns, err := provider.ListRepositoryRuns(ctx, repository, providers.ListRunsOptions{Created: baselineRange}, s.cfg.MaxHistoricalRuns)
	if err != nil {
		return nil, provider.MapError(err)
	}
	currentRuns = filterByWorkflow(currentRuns, workflow)
	baselineRuns = filterByWorkflow(baselineRuns, workflow)

	snapshot := analysis.BuildPerformanceSnapshot(repository, workflow, from, to, currentRuns, baselineRuns)
	return &snapshot, nil
}

func (s *Service) emitAudit(ctx context.Context, repository string, runID int64, reason, scope, outcome string) error {
	event := domain.AuditEvent{
		EventID:    fmt.Sprintf("evt_%d_%d", s.now().UnixNano(), runID),
		Tool:       "pipeline.rerun",
		Actor:      s.cfg.Actor,
		Repository: repository,
		RunID:      runID,
		Reason:     reason,
		Scope:      scope,
		Timestamp:  s.now().UTC().Format(time.RFC3339),
		Outcome:    outcome,
	}
	return s.audit.Append(ctx, event)
}

func (s *Service) resolveRunReference(ctx context.Context, ref RunReference, allowRepositoryOnly bool) (providers.Adapter, *providers.RunLocator, *domain.ToolError) {
	if strings.TrimSpace(ref.RunURL) != "" {
		provider, locator, toolErr := s.resolveProviderForRunURL(ref.Provider, ref.RunURL)
		if toolErr != nil {
			return nil, nil, toolErr
		}
		return provider, locator, nil
	}
	if ref.RunID <= 0 {
		if allowRepositoryOnly && strings.TrimSpace(ref.Repository) != "" {
			provider, toolErr := s.resolveProvider(ref.Provider)
			if toolErr != nil {
				return nil, nil, toolErr
			}
			locator, toolErr := s.resolveLatestFailedRun(ctx, provider, ref.Repository)
			if toolErr != nil {
				return nil, nil, toolErr
			}
			return provider, locator, nil
		}
		return nil, nil, domain.NewToolError(domain.ErrorCodeInvalidInput, "either run_url or run_id must be provided", "Provide a run_url, or both run_id and repository, or repository alone to use the latest failed run.", false, nil)
	}
	provider, toolErr := s.resolveProvider(ref.Provider)
	if toolErr != nil {
		return nil, nil, toolErr
	}
	repository, err := provider.ParseRepository(ref.Repository)
	if err != nil {
		return nil, nil, domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), fmt.Sprintf("Provide a valid repository identifier when using run_id with provider %q.", provider.ProviderID()), false, providerDetails(provider.ProviderID(), s.providers.ProviderIDs()))
	}
	return provider, &providers.RunLocator{Repository: repository, RunID: ref.RunID, RunURL: provider.RunURL(repository, ref.RunID)}, nil
}

func (s *Service) resolveLatestFailedRun(ctx context.Context, provider providers.Adapter, repository string) (*providers.RunLocator, *domain.ToolError) {
	repository, err := provider.ParseRepository(repository)
	if err != nil {
		return nil, domain.NewToolError(domain.ErrorCodeInvalidInput, err.Error(), s.repositoryRemediation(provider.ProviderID()), false, providerDetails(provider.ProviderID(), s.providers.ProviderIDs()))
	}

	runs, err := provider.ListRepositoryRuns(ctx, repository, providers.ListRunsOptions{PerPage: 1, Status: "failure"}, 1)
	if err != nil {
		return nil, provider.MapError(err)
	}
	if len(runs) == 0 {
		return nil, domain.NewToolError(
			domain.ErrorCodeInvalidInput,
			"No failed workflow runs were found for this repository.",
			fmt.Sprintf("Provide a specific run_url, or retry after a failed run exists for provider %q.", provider.ProviderID()),
			false,
			providerDetails(provider.ProviderID(), s.providers.ProviderIDs(), map[string]any{"repository": repository}),
		)
	}

	run := runs[0]
	runURL := strings.TrimSpace(run.RunURL)
	if runURL == "" {
		runURL = provider.RunURL(repository, run.ID)
	}

	return &providers.RunLocator{
		Repository: repository,
		RunID:      run.ID,
		RunURL:     runURL,
	}, nil
}

func (s *Service) resolveProvider(providerID string) (providers.Adapter, *domain.ToolError) {
	adapter, err := s.providers.Resolve(providerID)
	if err != nil {
		return nil, domain.NewToolError(
			domain.ErrorCodeInvalidInput,
			err.Error(),
			fmt.Sprintf("Omit provider to use the default %q, or choose one of: %s.", s.providers.DefaultProviderID(), strings.Join(s.providers.ProviderIDs(), ", ")),
			false,
			providerDetails(strings.TrimSpace(providerID), s.providers.ProviderIDs()),
		)
	}
	return adapter, nil
}

func (s *Service) resolveProviderForRunURL(providerID, runURL string) (providers.Adapter, *providers.RunLocator, *domain.ToolError) {
	adapter, locator, err := s.providers.ResolveRunURL(providerID, runURL)
	if err != nil {
		return nil, nil, domain.NewToolError(
			domain.ErrorCodeInvalidInput,
			err.Error(),
			s.runURLRemediation(strings.TrimSpace(providerID)),
			false,
			providerDetails(strings.TrimSpace(providerID), s.providers.ProviderIDs(), map[string]any{"run_url": runURL}),
		)
	}
	return adapter, locator, nil
}

func (s *Service) repositoryRemediation(providerID string) string {
	return fmt.Sprintf("Provide a valid repository identifier for provider %q.", providerID)
}

func (s *Service) runURLRemediation(providerID string) string {
	if providerID != "" {
		return fmt.Sprintf("Provide a valid pipeline run URL for provider %q.", providerID)
	}
	return fmt.Sprintf("Provide a valid pipeline run URL, or set provider explicitly. Supported providers: %s.", strings.Join(s.providers.ProviderIDs(), ", "))
}

func providerDetails(selectedProvider string, supportedProviders []string, extras ...map[string]any) map[string]any {
	details := map[string]any{
		"provider":            selectedProvider,
		"supported_providers": supportedProviders,
	}
	for _, extra := range extras {
		for key, value := range extra {
			details[key] = value
		}
	}
	return details
}

func normalizeRun(providerID string, run *providers.Run, repository, resolvedRunURL string, jobs []providers.Job) domain.PipelineRun {
	workflow := run.Name
	if strings.TrimSpace(workflow) == "" {
		workflow = run.DisplayTitle
	}
	var startedAt string
	var completedAt string
	var durationMS int64
	var queueTimeMS int64

	if run.RunStartedAt != nil {
		startedAt = run.RunStartedAt.UTC().Format(time.RFC3339)
		if run.UpdatedAt.After(*run.RunStartedAt) {
			durationMS = run.UpdatedAt.Sub(*run.RunStartedAt).Milliseconds()
		}
		if run.RunStartedAt.After(run.CreatedAt) {
			queueTimeMS = run.RunStartedAt.Sub(run.CreatedAt).Milliseconds()
		}
	}
	if run.UpdatedAt.After(run.CreatedAt) {
		completedAt = run.UpdatedAt.UTC().Format(time.RFC3339)
	}
	status := run.Status
	if strings.TrimSpace(run.Conclusion) != "" {
		status = run.Conclusion
	}
	runURL := strings.TrimSpace(run.RunURL)
	if runURL == "" {
		runURL = strings.TrimSpace(resolvedRunURL)
	}

	failureReason := ""
	for _, job := range jobs {
		if strings.EqualFold(job.Conclusion, "failure") {
			failureReason = "job failed: " + job.Name
			break
		}
	}

	return domain.PipelineRun{
		Provider:      providerID,
		Repository:    repository,
		Workflow:      workflow,
		Status:        status,
		StartedAt:     startedAt,
		CompletedAt:   completedAt,
		DurationMS:    durationMS,
		QueueTimeMS:   queueTimeMS,
		RunURL:        runURL,
		CommitSHA:     run.HeadSHA,
		RunID:         run.ID,
		Conclusion:    run.Conclusion,
		FailureReason: failureReason,
	}
}

func filterByWorkflow(runs []providers.Run, workflow string) []providers.Run {
	workflow = strings.TrimSpace(workflow)
	if workflow == "" {
		return runs
	}
	filtered := make([]providers.Run, 0, len(runs))
	for _, run := range runs {
		if strings.EqualFold(run.Name, workflow) || strings.EqualFold(run.DisplayTitle, workflow) {
			filtered = append(filtered, run)
		}
	}
	return filtered
}

func containsEnvProtectionEvidence(evidence []domain.EvidenceRef) bool {
	for _, item := range evidence {
		lower := strings.ToLower(item.Snippet)
		if strings.Contains(lower, "environment protection rules") || strings.Contains(lower, "not allowed to deploy") || strings.Contains(lower, "deployment was rejected") {
			return true
		}
	}
	return false
}

func buildEnvProtectionRecommendations(branch, environment string, policyNames []string) []domain.FixRecommendation {
	allowed := "the current deployment policy"
	if len(policyNames) > 0 {
		allowed = strings.Join(policyNames, ", ")
	}

	return []domain.FixRecommendation{
		{
			RecommendationID: "update-environment-branch-policy",
			Description:      fmt.Sprintf("Allow branch %q to deploy to %s, or broaden the environment branch policy.", branch, environment),
			ExpectedImpact:   "Removes the environment gate that is rejecting the job before any steps run.",
			Confidence:       0.93,
			References:       []string{"GitHub environment deployment branch policies"},
		},
		{
			RecommendationID: "dispatch-from-allowed-branch",
			Description:      fmt.Sprintf("Run the workflow from a branch that matches %s.", allowed),
			ExpectedImpact:   "Lets the deployment proceed without changing repository policy.",
			Confidence:       0.88,
		},
		{
			RecommendationID: "split-environments-by-branch",
			Description:      fmt.Sprintf("Route %q dispatches to a less restricted environment instead of %s.", branch, environment),
			ExpectedImpact:   "Preserves production protections while allowing validation runs from the main branch.",
			Confidence:       0.79,
		},
	}
}

func containsApprovalGateEvidence(evidence []domain.EvidenceRef) bool {
	for _, item := range evidence {
		if approvalRequiredPattern.MatchString(item.Snippet) {
			return true
		}
	}
	return false
}

func buildApprovalGateRecommendations(environment string) []domain.FixRecommendation {
	return []domain.FixRecommendation{
		{
			RecommendationID: "obtain-required-reviewer-approval",
			Description:      fmt.Sprintf("Request approval from the required reviewers for %s before re-running the deployment.", environment),
			ExpectedImpact:   "Satisfies the environment gate so the job can start executing steps.",
			Confidence:       0.92,
		},
		{
			RecommendationID: "adjust-required-reviewers-policy",
			Description:      fmt.Sprintf("Relax or narrow the required reviewer policy on %s if this workflow should not require manual approval.", environment),
			ExpectedImpact:   "Removes unnecessary manual blocking for lower-risk validation runs.",
			Confidence:       0.84,
		},
		{
			RecommendationID: "use-separate-nonproduction-environment",
			Description:      fmt.Sprintf("Route validation runs to a separate environment without approval gates instead of %s.", environment),
			ExpectedImpact:   "Preserves approval controls on protected environments while unblocking routine validation.",
			Confidence:       0.8,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ParseDateTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("timestamp is required")
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("timestamp %q must be RFC3339 or YYYY-MM-DD", value)
}
