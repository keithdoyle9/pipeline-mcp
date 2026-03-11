package gitlabapi

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

func TestClientGetPipelineAndJobs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/api/v4/projects/group%2Fapp/pipelines/10":
			_ = json.NewEncoder(w).Encode(Pipeline{ID: 10, Name: "ci", WebURL: "https://gitlab.example.com/group/app/-/pipelines/10"})
		case "/api/v4/projects/group%2Fapp/pipelines/10/jobs":
			_ = json.NewEncoder(w).Encode([]Job{{ID: 1, Name: "test", Status: "failed"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL+"/api/v4", "read-token", "", "test-agent", 5*time.Second)
	pipeline, err := client.GetPipeline(context.Background(), "group/app", 10)
	if err != nil {
		t.Fatalf("GetPipeline() error = %v", err)
	}
	if pipeline.ID != 10 {
		t.Fatalf("expected pipeline id 10, got %d", pipeline.ID)
	}

	jobs, err := client.ListPipelineJobs(context.Background(), "group/app", 10)
	if err != nil {
		t.Fatalf("ListPipelineJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
}

func TestClientListProjectPipelinesTranslatesCreatedRange(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]Pipeline{{ID: 10}})
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	_, err := client.ListProjectPipelines(context.Background(), "group/app", ListPipelinesOptions{
		PerPage:       1,
		CreatedAfter:  "2026-03-06T11:00:00Z",
		CreatedBefore: "2026-03-06T12:00:00Z",
		Status:        "failed",
		OrderBy:       "updated_at",
		Sort:          "desc",
	}, 1)
	if err != nil {
		t.Fatalf("ListProjectPipelines() error = %v", err)
	}
	for _, expected := range []string{"created_after=2026-03-06T11%3A00%3A00Z", "created_before=2026-03-06T12%3A00%3A00Z", "status=failed", "order_by=updated_at", "sort=desc"} {
		if !strings.Contains(gotQuery, expected) {
			t.Fatalf("expected query parameter %q, got %q", expected, gotQuery)
		}
	}
}

func TestClientDownloadJobTraceMapsNotFoundToLogsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	_, err := client.DownloadJobTrace(context.Background(), "group/app", 44, 1024)
	if !errors.Is(err, ErrLogsUnavailable) {
		t.Fatalf("expected ErrLogsUnavailable, got %v", err)
	}
}

func TestClientRetryPipelineRequiresWriteToken(t *testing.T) {
	client := NewClient("https://gitlab.com/api/v4", "read", "", "test-agent", 5*time.Second)
	err := client.RetryPipeline(context.Background(), "group/app", 12)
	if err != ErrWriteTokenRequired {
		t.Fatalf("expected ErrWriteTokenRequired, got %v", err)
	}
}

func TestClientRetriesTransientProviderFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) < 3 {
			http.Error(w, `{"message":"temporary upstream failure"}`, http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(Pipeline{ID: 42, Name: "ci"})
	}))
	defer server.Close()

	client := NewClient(server.URL, "read-token", "", "test-agent", 5*time.Second)
	pipeline, err := client.GetPipeline(context.Background(), "group/app", 42)
	if err != nil {
		t.Fatalf("GetPipeline() error = %v", err)
	}
	if pipeline == nil || pipeline.ID != 42 {
		t.Fatalf("expected successful retry result, got %#v", pipeline)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("expected 3 attempts, got %d", got)
	}
}

func TestClientRetriesTransientTransportFailures(t *testing.T) {
	var attempts atomic.Int32
	client := NewClient("https://gitlab.com/api/v4", "read-token", "", "test-agent", 5*time.Second)
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

	pipeline, err := client.GetPipeline(context.Background(), "group/app", 55)
	if err != nil {
		t.Fatalf("GetPipeline() error = %v", err)
	}
	if pipeline == nil || pipeline.ID != 55 {
		t.Fatalf("expected successful retry result, got %#v", pipeline)
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("expected 2 attempts, got %d", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
