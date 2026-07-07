package httpcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zinan-c/Poised/internal/core"
)

type Adapter struct {
	client *http.Client
}

type Payload struct {
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	ExpectedStatus int               `json:"expected_status"`
	Headers        map[string]string `json:"headers"`
}

func New() *Adapter {
	return &Adapter{
		client: http.DefaultClient,
	}
}

func NewWithClient(client *http.Client) *Adapter {
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{client: client}
}

func (adapter *Adapter) Name() string {
	return "http_check"
}

func (adapter *Adapter) Kind() string {
	return "monitor"
}

func (adapter *Adapter) Run(ctx context.Context, input core.RunInput) (core.RunResult, error) {
	var payload Payload
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		return core.RunResult{Status: core.RunStatusFailed}, err
	}
	if payload.URL == "" {
		return core.RunResult{Status: core.RunStatusFailed}, fmt.Errorf("url is required")
	}
	if payload.Method == "" {
		payload.Method = http.MethodGet
	}

	request, err := http.NewRequestWithContext(ctx, payload.Method, payload.URL, nil)
	if err != nil {
		return core.RunResult{Status: core.RunStatusFailed}, err
	}
	for name, value := range payload.Headers {
		request.Header.Set(name, value)
	}

	startedAt := time.Now()
	response, err := adapter.client.Do(request)
	durationMillis := time.Since(startedAt).Milliseconds()
	if err != nil {
		return core.RunResult{
			Status:  core.RunStatusFailed,
			Summary: "http request failed",
			Data: map[string]any{
				"url":             payload.URL,
				"duration_millis": durationMillis,
			},
		}, err
	}
	defer response.Body.Close()

	expectedStatus := payload.ExpectedStatus
	if expectedStatus == 0 {
		expectedStatus = http.StatusOK
	}

	status := core.RunStatusSuccess
	summary := "http check passed"
	if response.StatusCode != expectedStatus {
		status = core.RunStatusFailed
		summary = fmt.Sprintf("expected status %d, got %d", expectedStatus, response.StatusCode)
	}

	return core.RunResult{
		Status:  status,
		Summary: summary,
		Data: map[string]any{
			"url":             payload.URL,
			"method":          payload.Method,
			"expected_status": expectedStatus,
			"status_code":     response.StatusCode,
			"duration_millis": durationMillis,
		},
	}, nil
}
