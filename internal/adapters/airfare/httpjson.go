package airfare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

type JSONClient struct {
	baseURL string
	doer    HTTPDoer
	headers map[string]string
}

func NewJSONClient(baseURL string, doer HTTPDoer, headers map[string]string) (*JSONClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base url is required")
	}
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base url: %w", err)
	}
	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, fmt.Errorf("base url must include scheme and host")
	}

	if doer == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, fmt.Errorf("create cookie jar: %w", err)
		}
		doer = &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		}
	}

	return &JSONClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		doer:    doer,
		headers: cloneHeaders(headers),
	}, nil
}

func (client *JSONClient) Get(ctx context.Context, pathOrURL string, headers map[string]string) (*http.Response, []byte, error) {
	return client.do(ctx, http.MethodGet, pathOrURL, nil, headers)
}

func (client *JSONClient) PostJSON(ctx context.Context, pathOrURL string, payload any, headers map[string]string) (*http.Response, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal json payload: %w", err)
	}
	return client.do(ctx, http.MethodPost, pathOrURL, bytes.NewReader(body), headers)
}

func (client *JSONClient) do(ctx context.Context, method string, pathOrURL string, body io.Reader, headers map[string]string) (*http.Response, []byte, error) {
	requestURL, err := client.resolveURL(pathOrURL)
	if err != nil {
		return nil, nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, nil, err
	}
	for name, value := range client.headers {
		request.Header.Set(name, value)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}

	response, err := client.doer.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return response, nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response, responseBody, fmt.Errorf("unexpected status %d from %s", response.StatusCode, requestURL)
	}

	return response, responseBody, nil
}

func (client *JSONClient) resolveURL(pathOrURL string) (string, error) {
	if pathOrURL == "" {
		return "", fmt.Errorf("request path is required")
	}
	parsedURL, err := url.Parse(pathOrURL)
	if err != nil {
		return "", err
	}
	if parsedURL.IsAbs() {
		return parsedURL.String(), nil
	}
	if !strings.HasPrefix(pathOrURL, "/") {
		pathOrURL = "/" + pathOrURL
	}
	return client.baseURL + pathOrURL, nil
}

func cloneHeaders(headers map[string]string) map[string]string {
	cloned := make(map[string]string, len(headers))
	for name, value := range headers {
		cloned[name] = value
	}
	return cloned
}
