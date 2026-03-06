package githubapi

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrUnauthorized        = errors.New("github unauthorized")
	ErrRateLimited         = errors.New("github rate limited")
	ErrLogsUnavailable     = errors.New("github logs unavailable")
	ErrProviderUnavailable = errors.New("provider unavailable")
	ErrNotFound            = errors.New("resource not found")
	ErrWriteTokenRequired  = errors.New("write token required")
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
		return fmt.Sprintf("github api error: %d %s", e.Status, status)
	}
	return fmt.Sprintf("github api error: %d %s: %s", e.Status, status, e.Message)
}
