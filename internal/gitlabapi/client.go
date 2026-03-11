package gitlabapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func (c *Client) GetPipeline(ctx context.Context, projectPath string, pipelineID int64) (*Pipeline, error) {
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d", url.PathEscape(projectPath), pipelineID)
	var pipeline Pipeline
	if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &pipeline); err != nil {
		return nil, err
	}
	return &pipeline, nil
}

func (c *Client) ListPipelineJobs(ctx context.Context, projectPath string, pipelineID int64) ([]Job, error) {
	jobs := make([]Job, 0, 32)
	for page := 1; page <= 10; page++ {
		endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/jobs?per_page=%d&page=%d", url.PathEscape(projectPath), pipelineID, defaultPerPage, page)
		var pageJobs []Job
		if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &pageJobs); err != nil {
			return nil, err
		}
		if len(pageJobs) == 0 {
			break
		}
		jobs = append(jobs, pageJobs...)
		if len(pageJobs) < defaultPerPage {
			break
		}
	}
	return jobs, nil
}

func (c *Client) DownloadJobTrace(ctx context.Context, projectPath string, jobID int64, maxBytes int64) (string, error) {
	endpoint := fmt.Sprintf("/projects/%s/jobs/%d/trace", url.PathEscape(projectPath), jobID)
	body, _, err := c.doBytes(ctx, http.MethodGet, endpoint, nil, false)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return "", ErrLogsUnavailable
		}
		return "", err
	}
	if len(body) == 0 {
		return "", ErrLogsUnavailable
	}
	if maxBytes <= 0 || int64(len(body)) <= maxBytes {
		return string(body), nil
	}
	return string(body[:maxBytes]), nil
}

func (c *Client) ListProjectPipelines(ctx context.Context, projectPath string, opts ListPipelinesOptions, maxRuns int) ([]Pipeline, error) {
	if maxRuns <= 0 {
		maxRuns = defaultPerPage
	}
	if opts.PerPage <= 0 || opts.PerPage > defaultPerPage {
		opts.PerPage = defaultPerPage
	}

	pipelines := make([]Pipeline, 0, maxRuns)
	for page := 1; len(pipelines) < maxRuns && page <= 10; page++ {
		query := url.Values{}
		query.Set("per_page", strconv.Itoa(opts.PerPage))
		query.Set("page", strconv.Itoa(page))
		if opts.CreatedAfter != "" {
			query.Set("created_after", opts.CreatedAfter)
		}
		if opts.CreatedBefore != "" {
			query.Set("created_before", opts.CreatedBefore)
		}
		if opts.Status != "" {
			query.Set("status", opts.Status)
		}
		if opts.OrderBy != "" {
			query.Set("order_by", opts.OrderBy)
		}
		if opts.Sort != "" {
			query.Set("sort", opts.Sort)
		}

		endpoint := fmt.Sprintf("/projects/%s/pipelines?%s", url.PathEscape(projectPath), query.Encode())
		var pagePipelines []Pipeline
		if _, err := c.doJSON(ctx, http.MethodGet, endpoint, nil, false, &pagePipelines); err != nil {
			return nil, err
		}
		if len(pagePipelines) == 0 {
			break
		}
		pipelines = append(pipelines, pagePipelines...)
		if len(pagePipelines) < opts.PerPage {
			break
		}
	}
	if len(pipelines) > maxRuns {
		pipelines = pipelines[:maxRuns]
	}
	return pipelines, nil
}

func (c *Client) RetryPipeline(ctx context.Context, projectPath string, pipelineID int64) error {
	if strings.TrimSpace(c.writeToken) == "" {
		return ErrWriteTokenRequired
	}
	endpoint := fmt.Sprintf("/projects/%s/pipelines/%d/retry", url.PathEscape(projectPath), pipelineID)
	_, err := c.doJSON(ctx, http.MethodPost, endpoint, nil, true, nil)
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
	req.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if token := c.readToken; token != "" {
		req.Header.Set("PRIVATE-TOKEN", token)
	}
	if write && c.writeToken != "" {
		req.Header.Set("PRIVATE-TOKEN", c.writeToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, true, fmt.Errorf("gitlab request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, false, fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return responseBody, resp.Header, false, nil
	}

	apiErr := &APIError{
		Status:     resp.StatusCode,
		Message:    parseAPIErrorMessage(responseBody),
		RetryAfter: resp.Header.Get("Retry-After"),
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, resp.Header, false, fmt.Errorf("%w: %v", ErrUnauthorized, apiErr)
	case http.StatusNotFound:
		return nil, resp.Header, false, fmt.Errorf("%w: %v", ErrNotFound, apiErr)
	case http.StatusTooManyRequests:
		return nil, resp.Header, true, fmt.Errorf("%w: %v", ErrRateLimited, apiErr)
	default:
		if resp.StatusCode >= 500 {
			return nil, resp.Header, true, fmt.Errorf("%w: %v", ErrProviderUnavailable, apiErr)
		}
	}

	return nil, resp.Header, false, fmt.Errorf("gitlab api error: %v", apiErr)
}

func parseAPIErrorMessage(body []byte) string {
	if len(body) == 0 {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return strings.TrimSpace(string(body))
	}
	for _, key := range []string{"message", "error"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		default:
			encoded, err := json.Marshal(typed)
			if err == nil {
				return string(encoded)
			}
		}
	}
	return strings.TrimSpace(string(body))
}
