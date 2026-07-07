package store

import (
	"context"

	"github.com/zinan-c/Poised/internal/core"
)

type RunStore interface {
	SaveRun(ctx context.Context, run core.JobRun) error
	ListRuns(ctx context.Context, limit int) ([]core.JobRun, error)
}
