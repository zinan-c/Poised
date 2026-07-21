package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/runner"
	"github.com/zinan-c/Poised/internal/store"
)

const refreshInterval = 5 * time.Second

type Scheduler struct {
	source     store.JobSource
	runner     *runner.Runner
	logger     *slog.Logger
	runOnStart bool
	waitGroup  sync.WaitGroup
}

func New(source store.JobSource, runner *runner.Runner, logger *slog.Logger, runOnStart bool) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		source:     source,
		runner:     runner,
		logger:     logger,
		runOnStart: runOnStart,
	}
}

func (scheduler *Scheduler) Start(ctx context.Context) {
	scheduler.waitGroup.Add(1)
	go scheduler.loop(ctx)
}

func (scheduler *Scheduler) Wait() {
	scheduler.waitGroup.Wait()
}

func (scheduler *Scheduler) loop(ctx context.Context) {
	defer scheduler.waitGroup.Done()

	nextRuns := make(map[string]time.Time)
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()

	scheduler.logger.Info("database-backed scheduler started", "refresh_interval", refreshInterval.String())

	for {
		scheduler.tick(ctx, nextRuns)

		select {
		case <-ctx.Done():
			scheduler.logger.Info("database-backed scheduler stopped")
			return
		case <-ticker.C:
		}
	}
}

func (scheduler *Scheduler) tick(ctx context.Context, nextRuns map[string]time.Time) {
	jobs, err := scheduler.source.ListRunnableJobs(ctx)
	if err != nil {
		scheduler.logger.Error("list runnable jobs failed", "error", err)
		return
	}

	now := time.Now()
	seen := make(map[string]struct{}, len(jobs))
	for _, job := range jobs {
		key := scheduleKey(job)
		seen[key] = struct{}{}

		interval, err := time.ParseDuration(job.Interval)
		if err != nil {
			scheduler.logger.Error("skip job with invalid interval", "job_id", job.ID, "channel", job.Channel, "interval", job.Interval, "error", err)
			continue
		}

		nextRun, exists := nextRuns[key]
		if !exists {
			if scheduler.runOnStart {
				nextRun = now
			} else {
				nextRun = now.Add(interval)
			}
		}

		if now.Before(nextRun) {
			nextRuns[key] = nextRun
			continue
		}

		nextRuns[key] = now.Add(interval)
		go scheduler.runner.RunJob(ctx, job)
	}

	for key := range nextRuns {
		if _, exists := seen[key]; !exists {
			delete(nextRuns, key)
		}
	}
}

func scheduleKey(job core.JobSpec) string {
	return job.ID + "::" + job.Channel
}
