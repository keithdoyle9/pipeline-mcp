package githubapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestClientGetRunAndJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/app/actions/runs/10":
			_ = json.NewEncoder(w).Encode(WorkflowRun{ID: 10, Name: "ci", HTMLURL: "https://github.com/acme/app/actions/runs/10"})
		case "/repos/acme/app/actions/runs/10/jobs":
			_ = json.NewEncoder(w).Encode(listJobsResponse{TotalCount: 1, Jobs: []Job{{ID: 1, Name: "test", Conclusion: "failure"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	run, err := client.GetRun(context.Background(), "acme", "app", 10)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run.ID != 10 {
		t.Fatalf("expected run id 10, got %d", run.ID)
	}

	jobs, err := client.ListRunJobs(context.Background(), "acme", "app", 10)
	if err != nil {
		t.Fatalf("ListRunJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestClientRateLimitMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	_, err := client.GetRun(context.Background(), "acme", "app", 10)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ErrRateLimited.Error()) {
		t.Fatalf("expected rate limit error, got %v", err)
	}
}

func TestClientRerunRequiresWriteToken(t *testing.T) {
	client := NewClient("https://api.github.com", "read", "", "test-agent", 5*time.Second)
	err := client.Rerun(context.Background(), "acme", "app", 12, true)
	if err != ErrWriteTokenRequired {
		t.Fatalf("expected ErrWriteTokenRequired, got %v", err)
	}
}

func TestClientListRepositoryRunsIncludesStatusFilter(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(listRunsResponse{TotalCount: 1, WorkflowRuns: []WorkflowRun{{ID: 10}}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	_, err := client.ListRepositoryRuns(context.Background(), "acme", "app", ListRunsOptions{PerPage: 1, Status: "failure"}, 1)
	if err != nil {
		t.Fatalf("ListRepositoryRuns() error = %v", err)
	}
	if !strings.Contains(gotQuery, "status=failure") {
		t.Fatalf("expected status query parameter, got %q", gotQuery)
	}
}

func TestClientRetriesTransientProviderFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, `{"message":"temporary upstream failure"}`, http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(WorkflowRun{ID: 42, Name: "ci"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	run, err := client.GetRun(context.Background(), "acme", "app", 42)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run == nil || run.ID != 42 {
		t.Fatalf("expected successful retry result, got %#v", run)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestClientRetriesTransientTransportFailures(t *testing.T) {
	var attempts atomic.Int32
	client := NewClient("https://api.github.com", "read-token", "", "test-agent", 5*time.Second)
	client.httpClient = &http.Client{
		Timeout: 5 * time.Second,
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			if attempts.Add(1) == 1 {
				return nil, errors.New("temporary dial failure")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"id":55,"name":"ci"}`)),
			}, nil
		}),
	}

	run, err := client.GetRun(context.Background(), "acme", "app", 55)
	if err != nil {
		t.Fatalf("GetRun() error = %v", err)
	}
	if run == nil || run.ID != 55 {
		t.Fatalf("expected successful retry result, got %#v", run)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

func TestUnzipOrTextFallsBackToSafeLimitForInvalidMaxBytes(t *testing.T) {
	body := []byte("plain text logs")

	text, err := unzipOrText(body, -1)
	if err != nil {
		t.Fatalf("unzipOrText() error = %v", err)
	}
	if text != "plain text logs" {
		t.Fatalf("expected plain text logs, got %q", text)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
