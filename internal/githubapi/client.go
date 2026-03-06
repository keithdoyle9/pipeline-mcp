package githubapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultPerPage = 100
	maxAttempts    = 3
)

type Client struct {
	baseURL    string
	readToken  string
	writeToken string
	userAgent  string
	httpClient *http.Client
}

func NewClient(baseURL, readToken, writeToken, userAgent string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		readToken:  strings.TrimSpace(readToken),
		writeToken: strings.TrimSpace(writeToken),
		userAgent:  userAgent,
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) GetRun(ctx context.Context, owner, repo string, runID int64) (*WorkflowRun, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d", owner, repo, runID)
	var run WorkflowRun
	if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) ListRunJobs(ctx context.Context, owner, repo string, runID int64) ([]Job, error) {
	jobs := make([]Job, 0, 32)
	for page := 1; page <= 10; page++ {
		endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/jobs?per_page=%d&page=%d", owner, repo, runID, defaultPerPage, page)
		var resp listJobsResponse
		if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &resp); err != nil {
			return nil, err
		}
		if len(resp.Jobs) == 0 {
			break
		}
		jobs = append(jobs, resp.Jobs...)
		if len(resp.Jobs) < defaultPerPage {
			break
		}
	}
	return jobs, nil
}

func (c *Client) ListRepositoryRuns(ctx context.Context, owner, repo string, opts ListRunsOptions, maxRuns int) ([]WorkflowRun, error) {
	if maxRuns <= 0 {
		maxRuns = defaultPerPage
	}
	if opts.PerPage <= 0 || opts.PerPage > defaultPerPage {
		opts.PerPage = defaultPerPage
	}

	runs := make([]WorkflowRun, 0, maxRuns)
	for page := 1; len(runs) < maxRuns && page <= 10; page++ {
		query := url.Values{}
		query.Set("per_page", strconv.Itoa(opts.PerPage))
		query.Set("page", strconv.Itoa(page))
		if opts.Created != "" {
			query.Set("created", opts.Created)
		}
		if opts.Branch != "" {
			query.Set("branch", opts.Branch)
		}
		if opts.Event != "" {
			query.Set("event", opts.Event)
		}
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}

		endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs?%s", owner, repo, query.Encode())
		var resp listRunsResponse
		if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &resp); err != nil {
			return nil, err
		}
		if len(resp.WorkflowRuns) == 0 {
			break
		}
		runs = append(runs, resp.WorkflowRuns...)
		if len(resp.WorkflowRuns) < opts.PerPage {
			break
		}
	}
	if len(runs) > maxRuns {
		runs = runs[:maxRuns]
	}
	return runs, nil
}

func (c *Client) GetCheckRun(ctx context.Context, owner, repo string, checkRunID int64) (*CheckRun, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/check-runs/%d", owner, repo, checkRunID)
	var checkRun CheckRun
	if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &checkRun); err != nil {
		return nil, err
	}
	return &checkRun, nil
}

func (c *Client) GetCheckRunAnnotations(ctx context.Context, owner, repo string, checkRunID int64) ([]CheckRunAnnotation, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/check-runs/%d/annotations?per_page=%d", owner, repo, checkRunID, defaultPerPage)
	var annotations []CheckRunAnnotation
	if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &annotations); err != nil {
		return nil, err
	}
	return annotations, nil
}

func (c *Client) ListDeploymentBranchPolicies(ctx context.Context, owner, repo, environment string) ([]BranchPolicy, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/environments/%s/deployment-branch-policies", owner, repo, url.PathEscape(environment))
	var resp deploymentBranchPoliciesResponse
	if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &resp); err != nil {
		return nil, err
	}
	return resp.BranchPolicies, nil
}

func (c *Client) DownloadRunLogs(ctx context.Context, owner, repo string, runID int64, maxBytes int64) (string, error) {
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/logs", owner, repo, runID)
	body, _, err := c.doBytes(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		if err == ErrNotFound {
			return "", ErrLogsUnavailable
		}
		return "", err
	}
	if len(body) == 0 {
		return "", ErrLogsUnavailable
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}

	text, err := unzipOrText(body, maxBytes)
	if err != nil {
		return "", ErrLogsUnavailable
	}
	return text, nil
}

func (c *Client) Rerun(ctx context.Context, owner, repo string, runID int64, failedJobsOnly bool) error {
	if strings.TrimSpace(c.writeToken) == "" {
		return ErrWriteTokenRequired
	}
	endpoint := fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun", owner, repo, runID)
	if failedJobsOnly {
		endpoint = fmt.Sprintf("/repos/%s/%s/actions/runs/%d/rerun-failed-jobs", owner, repo, runID)
	}
	_, err := c.doJSON(ctx, http.MethodPost, endpoint, map[string]bool{"enable_debug_logging": false}, true, nil)
	return err
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, payload any, write bool, out any) (http.Header, error) {
	var body []byte
	var err error
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
	}

	respBody, headers, err := c.doBytes(ctx, method, endpoint, body, write)
	if err != nil {
		return headers, err
	}
	if out == nil || len(respBody) == 0 {
		return headers, nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return headers, fmt.Errorf("decode response: %w", err)
	}
	return headers, nil
}

func (c *Client) doBytes(ctx context.Context, method, endpoint string, body []byte, write bool) ([]byte, http.Header, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		responseBody, headers, retry, err := c.doOnce(ctx, method, endpoint, body, write)
		if err == nil {
			return responseBody, headers, nil
		}
		lastErr = err
		if !retry || attempt == maxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(time.Duration(attempt*attempt) * 200 * time.Millisecond):
		}
	}
	return nil, nil, lastErr
}

func (c *Client) doOnce(ctx context.Context, method, endpoint string, body []byte, write bool) ([]byte, http.Header, bool, error) {
	fullURL := c.baseURL + endpoint
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, nil, false, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := c.readToken; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if write && c.writeToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.writeToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, true, fmt.Errorf("github request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, false, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return responseBody, resp.Header, false, nil
	}

	msg := extractErrorMessage(responseBody)
	apiErr := &APIError{Status: resp.StatusCode, Message: msg, RetryAfter: resp.Header.Get("Retry-After")}

	if resp.StatusCode == http.StatusTooManyRequests || resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return nil, resp.Header, true, fmt.Errorf("%w: %v", ErrRateLimited, apiErr)
	}
	if resp.StatusCode >= 500 {
		return nil, resp.Header, true, fmt.Errorf("%w: %v", ErrProviderUnavailable, apiErr)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return nil, resp.Header, true, fmt.Errorf("%w: %v", ErrRateLimited, apiErr)
		}
		return nil, resp.Header, false, fmt.Errorf("%w: %v", ErrUnauthorized, apiErr)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, resp.Header, false, fmt.Errorf("%w: %v", ErrNotFound, apiErr)
	}

	return nil, resp.Header, false, fmt.Errorf("%w: %v", ErrProviderUnavailable, apiErr)
}

func extractErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return strings.TrimSpace(payload.Message)
	}
	if len(body) > 256 {
		return strings.TrimSpace(string(body[:256]))
	}
	return strings.TrimSpace(string(body))
}

func unzipOrText(body []byte, maxBytes int64) (string, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		if int64(len(body)) > maxBytes {
			body = body[:maxBytes]
		}
		return string(body), nil
	}

	buf := bytes.NewBuffer(nil)
	for _, file := range zipReader.File {
		if strings.HasSuffix(file.Name, "/") {
			continue
		}
		remaining := maxBytes - int64(buf.Len())
		if remaining <= 0 {
			break
		}
		h, err := file.Open()
		if err != nil {
			continue
		}
		_, _ = buf.WriteString("\n===== " + file.Name + " =====\n")
		remaining = maxBytes - int64(buf.Len())
		if remaining <= 0 {
			h.Close()
			break
		}
		if _, err := io.CopyN(buf, h, remaining); err != nil && err != io.EOF {
			h.Close()
			return "", err
		}
		h.Close()
		if int64(buf.Len()) >= maxBytes {
			break
		}
	}
	if buf.Len() == 0 {
		return "", fmt.Errorf("empty logs archive")
	}
	return buf.String(), nil
}
