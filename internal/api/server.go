package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/zinan-c/Poised/internal/adapters"
	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/runner"
	"github.com/zinan-c/Poised/internal/store"
)

type Server struct {
	jobs     []core.JobSpec
	registry *adapters.Registry
	runner   *runner.Runner
	store    store.RunStore
	logger   *slog.Logger
}

func NewServer(jobs []core.JobSpec, registry *adapters.Registry, runner *runner.Runner, store store.RunStore, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		jobs:     jobs,
		registry: registry,
		runner:   runner,
		store:    store,
		logger:   logger,
	}
}

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.handleHealth)
	mux.HandleFunc("GET /v1/adapters", server.handleAdapters)
	mux.HandleFunc("GET /v1/jobs", server.handleJobs)
	mux.HandleFunc("GET /v1/runs", server.handleRuns)
	mux.HandleFunc("POST /v1/jobs/", server.handleJobRun)
	return server.logRequests(mux)
}

func (server *Server) handleHealth(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, map[string]string{"status": "ok"})
}

func (server *Server) handleAdapters(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.registry.List())
}

func (server *Server) handleJobs(responseWriter http.ResponseWriter, request *http.Request) {
	writeJSON(responseWriter, http.StatusOK, server.jobs)
}

func (server *Server) handleRuns(responseWriter http.ResponseWriter, request *http.Request) {
	limit := 50
	if rawLimit := request.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(responseWriter, http.StatusBadRequest, "limit must be a number")
			return
		}
		limit = parsedLimit
	}

	runs, err := server.store.ListRuns(request.Context(), limit)
	if err != nil {
		writeError(responseWriter, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(responseWriter, http.StatusOK, runs)
}

func (server *Server) handleJobRun(responseWriter http.ResponseWriter, request *http.Request) {
	jobID, ok := parseRunPath(request.URL.Path)
	if !ok {
		writeError(responseWriter, http.StatusNotFound, "route not found")
		return
	}

	job, ok := server.findJob(jobID)
	if !ok {
		writeError(responseWriter, http.StatusNotFound, "job not found")
		return
	}

	run := server.runner.RunJob(request.Context(), job)
	writeJSON(responseWriter, http.StatusAccepted, run)
}

func (server *Server) findJob(jobID string) (core.JobSpec, bool) {
	for _, job := range server.jobs {
		if job.ID == jobID {
			return job, true
		}
	}
	return core.JobSpec{}, false
}

func (server *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		server.logger.Info("http request", "method", request.Method, "path", request.URL.Path)
		next.ServeHTTP(responseWriter, request)
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
