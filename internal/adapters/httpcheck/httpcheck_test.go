package httpcheck

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinan-c/Poised/internal/core"
)

func TestHTTPCheckPassesExpectedStatus(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.WriteHeader(http.StatusNoContent)
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		URL:            testServer.URL,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusNoContent,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "http-check",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Status != core.RunStatusSuccess {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}

func TestHTTPCheckFailsUnexpectedStatus(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		responseWriter.WriteHeader(http.StatusInternalServerError)
	}))
	defer testServer.Close()

	payload, err := json.Marshal(Payload{
		URL:            testServer.URL,
		Method:         http.MethodGet,
		ExpectedStatus: http.StatusOK,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := New().Run(context.Background(), core.RunInput{
		JobID:   "http-check",
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("run adapter: %v", err)
	}
	if result.Status != core.RunStatusFailed {
		t.Fatalf("unexpected status: %s", result.Status)
	}
}
