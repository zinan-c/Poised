package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/database"
	"github.com/zinan-c/Poised/internal/runner"
	"github.com/zinan-c/Poised/internal/store"
	"github.com/zinan-c/Poised/web"
)

const maxCollectionLimit = 500

type DatabaseChecker interface {
	Check(ctx context.Context) (database.CheckResult, error)
}

type JobView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Adapter   string `json:"adapter"`
	Enabled   bool   `json:"enabled"`
	Interval  string `json:"interval"`
	Timeout   string `json:"timeout"`
	Channel   string `json:"channel,omitempty"`
	ChannelID string `json:"channel_id,omitempty"`
}

type Server struct {
	registry        *adapters.Registry
	runner          *runner.Runner
	store           store.RunStore
	taskStore       store.TaskStore
	recordStore     store.RecordStore
	channelStore    store.ChannelStore
	jobSource       store.JobSource
	databaseChecker DatabaseChecker
	logger          *slog.Logger
}

func NewServer(registry *adapters.Registry, runner *runner.Runner, runStore store.RunStore, databaseChecker DatabaseChecker, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	taskStore, _ := runStore.(store.TaskStore)
	recordStore, _ := runStore.(store.RecordStore)
	channelStore, _ := runStore.(store.ChannelStore)
	jobSource, _ := runStore.(store.JobSource)
	return &Server{
		registry:        registry,
		runner:          runner,
		store:           runStore,
		taskStore:       taskStore,
		recordStore:     recordStore,
		channelStore:    channelStore,
		jobSource:       jobSource,
		databaseChecker: databaseChecker,
		logger:          logger,
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(web.Assets()))))
	mux.HandleFunc("GET /{$}", server.handleIndex)
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /v1/adapters", server.handleAdapters)
	mux.HandleFunc("GET /v1/jobs", server.handleJobs)
	mux.HandleFunc("GET /v1/tasks", server.handleTasks)
	mux.HandleFunc("POST /v1/tasks", server.handleTaskCreate)
	mux.HandleFunc("/v1/tasks/", server.handleTaskPath)
	mux.HandleFunc("GET /v1/runs", server.handleRuns)
	mux.HandleFunc("GET /v1/records", server.handleRecords)
	mux.HandleFunc("POST /v1/jobs/", server.handleJobRun)
	return server.logRequests(mux)
}

func (server *Server) handleIndex(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		writeError(responseWriter, http.StatusNotFound, "route not found")
		return
	}
	http.ServeFileFS(responseWriter, request, web.Files(), "index.html")
}

func (server *Server) handleHealth(responseWriter http.ResponseWriter, request *http.Request) {
	response := map[string]any{
		"status":   "ok",
		"database": "disabled",
	}

	if server.databaseChecker != nil {
		checkResult, err := server.databaseChecker.Check(request.Context())
		if err != nil {
			writeJSON(responseWriter, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"database": map[string]any{
					"error": err.Error(),
				},
			})
			return
		}
		response["database"] = checkResult
	}

	writeJSON(responseWriter, http.StatusOK, response)
}

func (server *Server) handleAdapters(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.registry.List())
}

func (server *Server) handleJobs(responseWriter http.ResponseWriter, request *http.Request) {
	if server.jobSource == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "job source is not available")
		return
	}
	jobs, err := server.jobSource.ListRunnableJobs(request.Context())
	if err != nil {
		server.logger.Error("list runnable jobs failed", "error", err)
		writeError(responseWriter, http.StatusInternalServerError, "list runnable jobs failed")
		return
	}
	views := make([]JobView, 0, len(jobs))
	for _, job := range jobs {
		views = append(views, JobView{
			ID:        job.ID,
			Name:      job.Name,
			Adapter:   job.Adapter,
			Enabled:   job.Enabled,
			Interval:  job.Interval,
			Timeout:   job.Timeout,
			Channel:   job.Channel,
			ChannelID: job.ChannelID,
		})
	}
	writeJSON(responseWriter, http.StatusOK, views)
}

func (server *Server) handleTasks(responseWriter http.ResponseWriter, request *http.Request) {
	if server.taskStore == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "task store is not available")
		return
	}

	limit, ok := parseLimit(responseWriter, request, 100)
	if !ok {
		return
	}
	tasks, err := server.taskStore.ListTasks(request.Context(), limit)
	if err != nil {
		server.logger.Error("list tasks failed", "error", err)
		writeError(responseWriter, http.StatusInternalServerError, "list tasks failed")
		return
	}

	writeJSON(responseWriter, http.StatusOK, tasks)
}

func (server *Server) handleTaskCreate(responseWriter http.ResponseWriter, request *http.Request) {
	if server.taskStore == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "task store is not available")
		return
	}

	var input store.TaskInput
	if !decodeJSON(responseWriter, request, &input) {
		return
	}
	task, err := server.taskStore.CreateTask(request.Context(), input)
	if err != nil {
		server.logger.Error("create task failed", "error", err)
		writeError(responseWriter, http.StatusBadRequest, "create task failed")
		return
	}
	writeJSON(responseWriter, http.StatusCreated, task)
}

func (server *Server) handleTaskPath(responseWriter http.ResponseWriter, request *http.Request) {
	taskKey, rest, ok := parseTaskPath(request.URL.Path)
	if !ok {
		writeError(responseWriter, http.StatusNotFound, "route not found")
		return
	}

	switch {
	case rest == "" && request.Method == http.MethodGet:
		server.handleTaskGet(responseWriter, request, taskKey)
	case rest == "" && request.Method == http.MethodPut:
		server.handleTaskUpdate(responseWriter, request, taskKey)
	case rest == "" && request.Method == http.MethodDelete:
		server.handleTaskDelete(responseWriter, request, taskKey)
	case rest == "pause" && request.Method == http.MethodPost:
		server.handleTaskStatus(responseWriter, request, taskKey, "paused")
	case rest == "resume" && request.Method == http.MethodPost:
		server.handleTaskStatus(responseWriter, request, taskKey, "active")
	case rest == "archive" && request.Method == http.MethodPost:
		server.handleTaskStatus(responseWriter, request, taskKey, "archived")
	case rest == "channels" && request.Method == http.MethodGet:
		server.handleChannels(responseWriter, request, taskKey)
	case rest == "channels" && request.Method == http.MethodPost:
		server.handleChannelCreate(responseWriter, request, taskKey)
	case strings.HasPrefix(rest, "channels/"):
		server.handleChannelPath(responseWriter, request, taskKey, strings.TrimPrefix(rest, "channels/"))
	default:
		writeError(responseWriter, http.StatusNotFound, "route not found")
	}
}

func (server *Server) handleTaskGet(responseWriter http.ResponseWriter, request *http.Request, taskKey string) {
	task, err := server.taskStore.GetTask(request.Context(), taskKey)
	if err != nil {
		writeError(responseWriter, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(responseWriter, http.StatusOK, task)
}

func (server *Server) handleTaskUpdate(responseWriter http.ResponseWriter, request *http.Request, taskKey string) {
	var input store.TaskInput
	if !decodeJSON(responseWriter, request, &input) {
		return
	}
	task, err := server.taskStore.UpdateTask(request.Context(), taskKey, input)
	if err != nil {
		server.logger.Error("update task failed", "task", taskKey, "error", err)
		writeError(responseWriter, http.StatusBadRequest, "update task failed")
		return
	}
	writeJSON(responseWriter, http.StatusOK, task)
}

func (server *Server) handleTaskDelete(responseWriter http.ResponseWriter, request *http.Request, taskKey string) {
	if err := server.taskStore.DeleteTask(request.Context(), taskKey); err != nil {
		writeError(responseWriter, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "deleted"})
}

func (server *Server) handleTaskStatus(responseWriter http.ResponseWriter, request *http.Request, taskKey string, status string) {
	task, err := server.taskStore.SetTaskStatus(request.Context(), taskKey, status)
	if err != nil {
		writeError(responseWriter, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(responseWriter, http.StatusOK, task)
}

func (server *Server) handleChannels(responseWriter http.ResponseWriter, request *http.Request, taskKey string) {
	if server.channelStore == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "channel store is not available")
		return
	}
	channels, err := server.channelStore.ListChannels(request.Context(), taskKey)
	if err != nil {
		writeError(responseWriter, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(responseWriter, http.StatusOK, channels)
}

func (server *Server) handleChannelCreate(responseWriter http.ResponseWriter, request *http.Request, taskKey string) {
	var input store.ChannelInput
	if !decodeJSON(responseWriter, request, &input) {
		return
	}
	channel, err := server.channelStore.CreateChannel(request.Context(), taskKey, input)
	if err != nil {
		server.logger.Error("create channel failed", "task", taskKey, "error", err)
		writeError(responseWriter, http.StatusBadRequest, "create channel failed")
		return
	}
	writeJSON(responseWriter, http.StatusCreated, channel)
}

func (server *Server) handleChannelPath(responseWriter http.ResponseWriter, request *http.Request, taskKey string, rest string) {
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" {
		writeError(responseWriter, http.StatusNotFound, "route not found")
		return
	}
	channel := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	switch {
	case action == "" && request.Method == http.MethodPut:
		var input store.ChannelInput
		if !decodeJSON(responseWriter, request, &input) {
			return
		}
		updated, err := server.channelStore.UpdateChannel(request.Context(), taskKey, channel, input)
		if err != nil {
			writeError(responseWriter, http.StatusBadRequest, "update channel failed")
			return
		}
		writeJSON(responseWriter, http.StatusOK, updated)
	case action == "" && request.Method == http.MethodDelete:
		if err := server.channelStore.DeleteChannel(request.Context(), taskKey, channel); err != nil {
			writeError(responseWriter, http.StatusNotFound, "channel not found")
			return
		}
		writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "deleted"})
	case action == "enable" && request.Method == http.MethodPost:
		updated, err := server.channelStore.SetChannelEnabled(request.Context(), taskKey, channel, true)
		if err != nil {
			writeError(responseWriter, http.StatusNotFound, "channel not found")
			return
		}
		writeJSON(responseWriter, http.StatusOK, updated)
	case action == "disable" && request.Method == http.MethodPost:
		updated, err := server.channelStore.SetChannelEnabled(request.Context(), taskKey, channel, false)
		if err != nil {
			writeError(responseWriter, http.StatusNotFound, "channel not found")
			return
		}
		writeJSON(responseWriter, http.StatusOK, updated)
	default:
		writeError(responseWriter, http.StatusNotFound, "route not found")
	}
}

func (server *Server) handleRuns(responseWriter http.ResponseWriter, request *http.Request) {
	limit, ok := parseLimit(responseWriter, request, 50)
	if !ok {
		return
	}
	runs, err := server.store.ListRuns(request.Context(), limit)
	if err != nil {
		server.logger.Error("list runs failed", "error", err)
		writeError(responseWriter, http.StatusInternalServerError, "list runs failed")
		return
	}

	writeJSON(responseWriter, http.StatusOK, runs)
}

func (server *Server) handleRecords(responseWriter http.ResponseWriter, request *http.Request) {
	if server.recordStore == nil {
		writeError(responseWriter, http.StatusServiceUnavailable, "record store is not available")
		return
	}

	limit, ok := parseLimit(responseWriter, request, 100)
	if !ok {
		return
	}
	records, err := server.recordStore.ListRecords(request.Context(), limit)
	if err != nil {
		server.logger.Error("list records failed", "error", err)
		writeError(responseWriter, http.StatusInternalServerError, "list records failed")
		return
	}

	writeJSON(responseWriter, http.StatusOK, records)
}

func (server *Server) handleJobRun(responseWriter http.ResponseWriter, request *http.Request) {
	jobID, ok := parseRunPath(request.URL.Path)
	if !ok {
		writeError(responseWriter, http.StatusNotFound, "route not found")
		return
	}

	jobs, err := server.findRunnableJobs(request.Context(), jobID, request.URL.Query().Get("channel"))
	if err != nil {
		server.logger.Error("find runnable jobs failed", "job_id", jobID, "error", err)
		writeError(responseWriter, http.StatusInternalServerError, "find runnable jobs failed")
		return
	}
	if len(jobs) == 0 {
		writeError(responseWriter, http.StatusNotFound, "runnable job not found")
		return
	}

	runs := make([]core.JobRun, 0, len(jobs))
	for _, job := range jobs {
		runs = append(runs, server.runner.RunJob(request.Context(), job))
	}
	writeJSON(responseWriter, http.StatusOK, map[string]any{"runs": runs})
}

func (server *Server) findRunnableJobs(ctx context.Context, jobID string, channel string) ([]core.JobSpec, error) {
	if server.jobSource == nil {
		return nil, fmt.Errorf("job source is not available")
	}
	jobs, err := server.jobSource.ListRunnableJobs(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]core.JobSpec, 0)
	for _, job := range jobs {
		if job.ID != jobID {
			continue
		}
		if channel != "" && job.Channel != channel {
			continue
		}
		matched = append(matched, job)
	}
	return matched, nil
}

func (server *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		startedAt := time.Now()
		recorder := &statusRecorder{ResponseWriter: responseWriter, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, request)
		server.logger.Info("http request",
			"method", request.Method,
			"path", request.URL.Path,
			"status", recorder.statusCode,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	})
}

func parseRunPath(path string) (string, bool) {
	if !strings.HasPrefix(path, "/v1/jobs/") {
		return "", false
	}

	trimmed := strings.TrimPrefix(path, "/v1/jobs/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "runs" {
		return "", false
	}

	return parts[0], true
}

func parseTaskPath(path string) (string, string, bool) {
	if !strings.HasPrefix(path, "/v1/tasks/") {
		return "", "", false
	}
	trimmed := strings.Trim(strings.TrimPrefix(path, "/v1/tasks/"), "/")
	if trimmed == "" {
		return "", "", false
	}
	parts := strings.SplitN(trimmed, "/", 2)
	taskKey := parts[0]
	rest := ""
	if len(parts) == 2 {
		rest = parts[1]
	}
	return taskKey, rest, true
}

func parseLimit(responseWriter http.ResponseWriter, request *http.Request, defaultLimit int) (int, bool) {
	limit := defaultLimit
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(responseWriter, http.StatusBadRequest, "limit must be a number")
			return 0, false
		}
		if parsedLimit < 1 {
			writeError(responseWriter, http.StatusBadRequest, "limit must be at least 1")
			return 0, false
		}
		if parsedLimit > maxCollectionLimit {
			writeError(responseWriter, http.StatusBadRequest, "limit must be at most 500")
			return 0, false
		}
		limit = parsedLimit
	}
	return limit, true
}

func decodeJSON(responseWriter http.ResponseWriter, request *http.Request, target any) bool {
	defer request.Body.Close()
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(responseWriter, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (recorder *statusRecorder) WriteHeader(statusCode int) {
	recorder.statusCode = statusCode
	recorder.ResponseWriter.WriteHeader(statusCode)
}

func writeJSON(responseWriter http.ResponseWriter, statusCode int, payload any) {
	responseWriter.Header().Set("Content-Type", "application/json")
	responseWriter.WriteHeader(statusCode)
	encoder := json.NewEncoder(responseWriter)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(payload); err != nil {
		slog.Default().Error("write json failed", "error", err)
	}
}

func writeError(responseWriter http.ResponseWriter, statusCode int, message string) {
	writeJSON(responseWriter, statusCode, map[string]string{"error": message})
}
