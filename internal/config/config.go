package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/zinan-c/Poised/internal/core"
)

type Config struct {
	HTTP      HTTPConfig      `json:"http"`
	Scheduler SchedulerConfig `json:"scheduler"`
	Jobs      []core.JobSpec  `json:"jobs"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type SchedulerConfig struct {
	RunOnStart bool `json:"run_on_start"`
}

func Load(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var config Config
	if err := json.Unmarshal(content, &config); err != nil {
		return Config{}, err
	}

	if config.HTTP.Addr == "" {
		config.HTTP.Addr = "127.0.0.1:8080"
	}
	if envAddr := os.Getenv("POISED_HTTP_ADDR"); envAddr != "" {
		config.HTTP.Addr = envAddr
	}

	if err := validate(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

func validate(config Config) error {
	seenJobs := make(map[string]struct{}, len(config.Jobs))
	for _, job := range config.Jobs {
		if job.ID == "" {
			return fmt.Errorf("job id is required")
		}
		if job.Adapter == "" {
			return fmt.Errorf("job %q adapter is required", job.ID)
		}
		if _, exists := seenJobs[job.ID]; exists {
			return fmt.Errorf("job %q is duplicated", job.ID)
		}
		seenJobs[job.ID] = struct{}{}
	}

	return nil
}
