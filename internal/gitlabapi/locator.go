package gitlabapi

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type RunLocator struct {
	ProjectPath string
	RunID       int64
	RunURL      string
}

func (r RunLocator) Repository() string {
	return r.ProjectPath
}

func ParseProjectPath(project string) (string, error) {
	project = strings.Trim(strings.TrimSpace(project), "/")
	if project == "" {
		return "", fmt.Errorf("repository is required")
	}
	parts := strings.Split(project, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("repository must be in group/project format")
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("repository contains an empty path segment")
		}
	}
	return strings.Join(parts, "/"), nil
}

func ParseRunURL(raw string) (*RunLocator, error) {
	return ParseRunURLForBase(raw, "https://gitlab.com/api/v4")
}

func ParseRunURLForBase(raw, baseURL string) (*RunLocator, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("run_url is required")
	}

	runURL, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("parse run url: %w", err)
	}

	webBase, err := webBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(runURL.Host, webBase.Host) {
		return nil, fmt.Errorf("unsupported host %q", runURL.Host)
	}

	cleanPath := strings.Trim(path.Clean(runURL.Path), "/")
	basePath := strings.Trim(path.Clean(webBase.Path), "/")
	relativePath := cleanPath
	if basePath != "" && basePath != "." {
		prefix := basePath + "/"
		if !strings.HasPrefix(cleanPath, prefix) {
			return nil, fmt.Errorf("unsupported run url path")
		}
		relativePath = strings.TrimPrefix(cleanPath, prefix)
	}

	segments := strings.Split(relativePath, "/")
	if len(segments) < 4 || segments[len(segments)-2] != "pipelines" || segments[len(segments)-3] != "-" {
		return nil, fmt.Errorf("unsupported run url path")
	}

	runID, err := strconv.ParseInt(segments[len(segments)-1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse run id: %w", err)
	}
	projectPath, err := ParseProjectPath(strings.Join(segments[:len(segments)-3], "/"))
	if err != nil {
		return nil, err
	}

	return &RunLocator{
		ProjectPath: projectPath,
		RunID:       runID,
		RunURL:      strings.TrimRight(webBase.String(), "/") + "/" + projectPath + "/-/pipelines/" + strconv.FormatInt(runID, 10),
	}, nil
}

func RunURLForBase(projectPath string, runID int64, baseURL string) string {
	webBase, err := webBaseURL(baseURL)
	if err != nil {
		return ""
	}
	projectPath, err = ParseProjectPath(projectPath)
	if err != nil {
		return ""
	}
	return strings.TrimRight(webBase.String(), "/") + "/" + projectPath + "/-/pipelines/" + strconv.FormatInt(runID, 10)
}

func webBaseURL(baseURL string) (*url.URL, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return nil, fmt.Errorf("parse gitlab api base url: %w", err)
	}
	if strings.TrimSpace(base.Host) == "" {
		return nil, fmt.Errorf("gitlab api base url host is required")
	}

	webBase := *base
	webBase.RawQuery = ""
	webBase.Fragment = ""
	webBase.Path = strings.TrimSuffix(webBase.Path, "/api/v4")
	webBase.Path = strings.TrimRight(webBase.Path, "/")
	return &webBase, nil
}
