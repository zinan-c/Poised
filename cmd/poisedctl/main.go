package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	baseURL := flag.String("addr", "http://127.0.0.1:8080", "poised api base url")
	flag.Parse()

	if flag.NArg() == 0 {
		usage()
		os.Exit(2)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	command := flag.Arg(0)

	var err error
	switch command {
	case "adapters":
		err = get(client, *baseURL, "/v1/adapters")
	case "jobs":
		err = get(client, *baseURL, "/v1/jobs")
	case "runs":
		err = get(client, *baseURL, "/v1/runs")
	case "run":
		if flag.NArg() < 2 {
			fmt.Fprintln(os.Stderr, "missing job id")
			os.Exit(2)
		}
		jobID := flag.Arg(1)
		err = post(client, *baseURL, "/v1/jobs/"+jobID+"/runs")
	default:
		usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: poisedctl [-addr http://127.0.0.1:8080] adapters|jobs|runs|run <job-id>")
}

func get(client *http.Client, baseURL string, path string) error {
	request, err := http.NewRequest(http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	return do(client, request)
}

func post(client *http.Client, baseURL string, path string) error {
	request, err := http.NewRequest(http.MethodPost, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	return do(client, request)
}

func do(client *http.Client, request *http.Request) error {
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", response.Status, strings.TrimSpace(string(body)))
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		fmt.Print(string(body))
		return nil
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}
