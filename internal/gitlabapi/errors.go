package gitlabapi

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrUnauthorized         = errors.New("gitlab unauthorized")
	ErrRateLimited          = errors.New("gitlab rate limited")
	ErrLogsUnavailable      = errors.New("gitlab logs unavailable")
	ErrProviderUnavailable  = errors.New("provider unavailable")
	ErrNotFound             = errors.New("resource not found")
	ErrWriteTokenRequired   = errors.New("write token required")
	ErrFullRerunUnsupported = errors.New("gitlab full pipeline rerun is unsupported")
	ErrCheckRunUnsupported  = errors.New("gitlab check run metadata is unsupported")
)

type APIError struct {
	Status     int
	Message    string
	RetryAfter string
}

func (e *APIError) Error() string {
	status := http.StatusText(e.Status)
	if status == "" {
		status = "unknown"
	}
	if e.Message == "" {
		return fmt.Sprintf("gitlab api error: %d %s", e.Status, status)
	}
	return fmt.Sprintf("gitlab api error: %d %s: %s", e.Status, status, e.Message)
}
