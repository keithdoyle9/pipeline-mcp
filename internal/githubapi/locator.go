package githubapi

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type RunLocator struct {
	Owner  string
	Repo   string
	RunID  int64
	RunURL string
}

func (r RunLocator) Repository() string {
	return fmt.Sprintf("%s/%s", r.Owner, r.Repo)
}

func ParseRunURL(raw string) (*RunLocator, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("run_url is required")
	}

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse run url: %w", err)
	}

	if !strings.EqualFold(u.Host, "github.com") {
		return nil, fmt.Errorf("unsupported host %q", u.Host)
	}

	segments := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
	if len(segments) < 5 || segments[2] != "actions" || segments[3] != "runs" {
		return nil, fmt.Errorf("unsupported run url path")
	}

	runID, err := strconv.ParseInt(segments[4], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse run id: %w", err)
	}

	return &RunLocator{
		Owner:  segments[0],
		Repo:   segments[1],
		RunID:  runID,
		RunURL: fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", segments[0], segments[1], runID),
	}, nil
}

func ParseRepository(repo string) (owner string, name string, err error) {
	repo = strings.TrimSpace(repo)
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository must be in owner/repo format")
	}
	return parts[0], parts[1], nil
}

func ParseCheckRunURL(raw string) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("check_run_url is required")
	}

	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("parse check run url: %w", err)
	}

	segments := strings.Split(strings.Trim(path.Clean(u.Path), "/"), "/")
	if len(segments) < 5 || segments[3] != "check-runs" {
		return 0, fmt.Errorf("unsupported check run url path")
	}

	checkRunID, err := strconv.ParseInt(segments[4], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse check run id: %w", err)
	}

	return checkRunID, nil
}
