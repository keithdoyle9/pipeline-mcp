package gitlabapi

import "time"

type Pipeline struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	WebURL         string     `json:"web_url"`
	SHA            string     `json:"sha"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StartedAt      *time.Time `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	Duration       *float64   `json:"duration"`
	QueuedDuration *float64   `json:"queued_duration"`
}

type Job struct {
	ID         int64      `json:"id"`
	Status     string     `json:"status"`
	Stage      string     `json:"stage"`
	Name       string     `json:"name"`
	Ref        string     `json:"ref"`
	WebURL     string     `json:"web_url"`
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

type ListPipelinesOptions struct {
	PerPage       int
	Page          int
	CreatedAfter  string
	CreatedBefore string
	Status        string
	OrderBy       string
	Sort          string
}
