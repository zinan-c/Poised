package core

import (
	"encoding/json"
	"time"
)

type JobSpec struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Adapter  string          `json:"adapter"`
	Enabled  bool            `json:"enabled"`
	Interval string          `json:"interval"`
	Timeout  string          `json:"timeout"`
	Payload  json.RawMessage `json:"payload"`
}

type RunInput struct {
	JobID    string          `json:"job_id"`
	Payload  json.RawMessage `json:"payload"`
	Metadata map[string]any  `json:"metadata,omitempty"`
}

type RunStatus string

const (
	RunStatusSuccess  RunStatus = "success"
	RunStatusFailed   RunStatus = "failed"
	RunStatusCanceled RunStatus = "canceled"
)

type RunResult struct {
	Status  RunStatus      `json:"status"`
	Summary string         `json:"summary,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type JobRun struct {
	ID             string    `json:"id"`
	JobID          string    `json:"job_id"`
	Adapter        string    `json:"adapter"`
	Status         RunStatus `json:"status"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	DurationMillis int64     `json:"duration_millis"`
	Summary        string    `json:"summary,omitempty"`
	Error          string    `json:"error,omitempty"`
	Result         RunResult `json:"result"`
}

type AdapterInfo struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}
