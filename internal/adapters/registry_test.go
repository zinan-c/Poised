package adapters

import (
	"context"
	"testing"

	"github.com/zinan-c/Poised/internal/core"
)

type testAdapter struct{}

func (adapter testAdapter) Name() string {
	return "test"
}

func (adapter testAdapter) Kind() string {
	return "test"
}

func (adapter testAdapter) Run(ctx context.Context, input core.RunInput) (core.RunResult, error) {
	return core.RunResult{Status: core.RunStatusSuccess}, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testAdapter{}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}

	adapter, exists := registry.Get("test")
	if !exists {
		t.Fatal("adapter not found")
	}
	if adapter.Kind() != "test" {
		t.Fatalf("unexpected kind: %s", adapter.Kind())
	}
}

func TestRegistryRejectsDuplicate(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(testAdapter{}); err != nil {
		t.Fatalf("register adapter: %v", err)
	}
	if err := registry.Register(testAdapter{}); err == nil {
		t.Fatal("expected duplicate adapter error")
	}
}
