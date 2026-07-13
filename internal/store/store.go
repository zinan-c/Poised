package store

import (
	"context"

	"github.com/zinan-c/Poised/internal/core"
)

type RunStore interface {
	SaveRun(ctx context.Context, run core.JobRun) error
	ListRuns(ctx context.Context, limit int) ([]core.JobRun, error)
}

type TaskStore interface {
	UpsertTask(ctx context.Context, job core.JobSpec) (MonitorTask, error)
	ListTasks(ctx context.Context, limit int) ([]MonitorTask, error)
}

type RecordStore interface {
	ListRecords(ctx context.Context, limit int) ([]MonitorRecord, error)
}
