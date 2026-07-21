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
	CreateTask(ctx context.Context, input TaskInput) (MonitorTask, error)
	GetTask(ctx context.Context, key string) (MonitorTask, error)
	ListTasks(ctx context.Context, limit int) ([]MonitorTask, error)
	UpdateTask(ctx context.Context, key string, input TaskInput) (MonitorTask, error)
	SetTaskStatus(ctx context.Context, key string, status string) (MonitorTask, error)
	ArchiveTask(ctx context.Context, key string) (MonitorTask, error)
	DeleteTask(ctx context.Context, key string) error
}

type RecordStore interface {
	ListRecords(ctx context.Context, limit int) ([]MonitorRecord, error)
}

type ChannelStore interface {
	CreateChannel(ctx context.Context, taskKey string, input ChannelInput) (MonitorTaskChannel, error)
	ListChannels(ctx context.Context, taskKey string) ([]MonitorTaskChannel, error)
	UpdateChannel(ctx context.Context, taskKey string, channel string, input ChannelInput) (MonitorTaskChannel, error)
	SetChannelEnabled(ctx context.Context, taskKey string, channel string, enabled bool) (MonitorTaskChannel, error)
	DeleteChannel(ctx context.Context, taskKey string, channel string) error
}

type JobSource interface {
	ListRunnableJobs(ctx context.Context) ([]core.JobSpec, error)
}
