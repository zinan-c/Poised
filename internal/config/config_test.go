package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRejectsUnknownFields(t *testing.T) {
	path := writeConfig(t, `{
  "http": { "addr": "127.0.0.1:8080" },
  "database": { "url": "postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable" },
  "scheduler": { "run_on_start": true },
  "unknown": true,
  "jobs": []
}`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestLoadRejectsEnabledJobWithoutInterval(t *testing.T) {
	path := writeConfig(t, `{
  "http": { "addr": "127.0.0.1:8080" },
  "database": { "url": "postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable" },
  "scheduler": { "run_on_start": true },
  "jobs": [{
    "id": "example",
    "name": "Example",
    "adapter": "echo",
    "enabled": true,
    "timeout": "10s",
    "payload": {}
  }]
}`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "interval is required") {
		t.Fatalf("expected interval error, got %v", err)
	}
}

func TestLoadRejectsFractionalDuration(t *testing.T) {
	path := writeConfig(t, `{
  "http": { "addr": "127.0.0.1:8080" },
  "database": { "url": "postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable" },
  "scheduler": { "run_on_start": true },
  "jobs": [{
    "id": "example",
    "name": "Example",
    "adapter": "echo",
    "enabled": true,
    "interval": "1500ms",
    "timeout": "10s",
    "payload": {}
  }]
}`)

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "whole number of seconds") {
		t.Fatalf("expected whole-second duration error, got %v", err)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}
