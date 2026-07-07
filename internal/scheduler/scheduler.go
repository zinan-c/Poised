package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/zinan-c/Poised/internal/core"
	"github.com/zinan-c/Poised/internal/runner"
)

type Scheduler struct {
	jobs       []core.JobSpec
	runner     *runner.Runner
	logger     *slog.Logger
	runOnStart bool
	waitGroup  sync.WaitGroup
}

func New(jobs []core.JobSpec, runner *runner.Runner, logger *slog.Logger, runOnStart bool) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		jobs:       jobs,
		runner:     runner,
		logger:     logger,
		runOnStart: runOnStart,
	}
}

func (scheduler *Scheduler) Start(ctx context.Context) {
	for _, job := range scheduler.jobs {
		if !job.Enabled {
			continue
		}

		interval, err := time.ParseDuration(job.Interval)
		if err != nil {
			scheduler.logger.Error("skip job with invalid interval", "job_id", job.ID, "interval", job.Interval, "error", err)
			continue
		}

		scheduler.waitGroup.Add(1)
		go scheduler.loop(ctx, job, interval)
	}
}

func (scheduler *Scheduler) Wait() {
	scheduler.waitGroup.Wait()
}

func (scheduler *Scheduler) loop(ctx context.Context, job core.JobSpec, interval time.Duration) {
	defer scheduler.waitGroup.Done()

	scheduler.logger.Info("scheduled job started", "job_id", job.ID, "interval", interval.String())

	if scheduler.runOnStart {
		scheduler.runner.RunJob(ctx, job)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			scheduler.logger.Info("scheduled job stopped", "job_id", job.ID)
			return
		case <-ticker.C:
			scheduler.runner.RunJob(ctx, job)
		}
	}
}
