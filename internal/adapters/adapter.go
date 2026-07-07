package adapters

import (
	"context"

	"github.com/zinan-c/Poised/internal/core"
)

type Adapter interface {
	Name() string
	Kind() string
	Run(ctx context.Context, input core.RunInput) (core.RunResult, error)
}
