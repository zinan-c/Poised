package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/adapters/examples/echo"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/runner"
	"github.com/zinan-c/Poised/internal/store"
)

func TestParseLimitRejectsInvalidBounds(t *testing.T) {
	for _, rawLimit := range []string{"0", "-1", "501"} {
		request := httptest.NewRequest(http.MethodGet, "/v1/runs?limit="+rawLimit, nil)
		response := httptest.NewRecorder()

		if _, ok := parseLimit(response, request, 50); ok {
			t.Fatalf("expected limit %q to be rejected", rawLimit)
		}
		if response.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status for %q: %d", rawLimit, response.Code)
		}
	}
}

func TestJobsEndpointReturnsSanitizedJobs(t *testing.T) {
	registry := adapters.NewRegistry()
	if err := registry.Register(echo.New()); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	runStore := &apiTestStore{
		MemoryRunStore: store.NewMemoryRunStore(),
		jobs: []core.JobSpec{{
			ID:       "secret-job",
			Name:     "Secret Job",
			Adapter:  "echo",
			Enabled:  true,
			Interval: "30s",
			Timeout:  "10s",
			Payload:  []byte(`{"token":"secret"}`),
		}},
	}
	jobRunner := runner.New(registry, runStore, nil)
	server := NewServer(registry, jobRunner, runStore, nil, nil)

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.Code)
	}

	var jobs []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if _, exists := jobs[0]["payload"]; exists {
		t.Fatalf("jobs response should not include payload: %s", response.Body.String())
	}
}

type apiTestStore struct {
	*store.MemoryRunStore
	jobs []core.JobSpec
}

func (store *apiTestStore) ListRunnableJobs(ctx context.Context) ([]core.JobSpec, error) {
	return store.jobs, nil
}
