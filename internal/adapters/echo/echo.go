package echo

import (
	"context"
	"encoding/json"

	"github.com/zinan-c/Poised/internal/core"
)

type Adapter struct{}

func New() *Adapter {
	return &Adapter{}
}

func (adapter *Adapter) Name() string {
	return "echo"
}

func (adapter *Adapter) Kind() string {
	return "utility"
}

func (adapter *Adapter) Run(ctx context.Context, input core.RunInput) (core.RunResult, error) {
	select {
	case <-ctx.Done():
		return core.RunResult{Status: core.RunStatusCanceled}, ctx.Err()
	default:
	}

	var payload map[string]any
	if len(input.Payload) > 0 {
		if err := json.Unmarshal(input.Payload, &payload); err != nil {
			return core.RunResult{Status: core.RunStatusFailed}, err
		}
	}
	if payload == nil {
		payload = map[string]any{}
	}

	return core.RunResult{
		Status:  core.RunStatusSuccess,
		Summary: "echo adapter completed",
		Data: map[string]any{
			"job_id":  input.JobID,
			"payload": payload,
		},
	}, nil
}
