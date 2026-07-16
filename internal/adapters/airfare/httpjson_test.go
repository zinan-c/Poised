package airfare

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestJSONClientRejectsOversizedResponse(t *testing.T) {
	client, err := NewJSONClient("https://example.test", fixedDoer{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(strings.Repeat("x", maxResponseBodyBytes+1))),
		},
	}, nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	_, _, err = client.Get(context.Background(), "/too-large", nil)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized response error, got %v", err)
	}
}

type fixedDoer struct {
	response *http.Response
	err      error
}

func (doer fixedDoer) Do(request *http.Request) (*http.Response, error) {
	return doer.response, doer.err
}
