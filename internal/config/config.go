package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/zinan-c/Poised/internal/core"
)

var jobIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type Config struct {
	HTTP      HTTPConfig      `json:"http"`
	Database  DatabaseConfig  `json:"database"`
	Scheduler SchedulerConfig `json:"scheduler"`
	Jobs      []core.JobSpec  `json:"jobs"`
}

type HTTPConfig struct {
	Addr string `json:"addr"`
}

type DatabaseConfig struct {
	URL         string `json:"url"`
	AutoMigrate bool   `json:"auto_migrate"`
	MaxConns    int32  `json:"max_conns"`
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
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, err
	}

	if config.HTTP.Addr == "" {
		config.HTTP.Addr = "127.0.0.1:8080"
	}
	if envAddr := os.Getenv("POISED_HTTP_ADDR"); envAddr != "" {
		config.HTTP.Addr = envAddr
	}
	if envURL := os.Getenv("POISED_DATABASE_URL"); envURL != "" {
		config.Database.URL = envURL
	}
	if envAutoMigrate := os.Getenv("POISED_DATABASE_AUTO_MIGRATE"); envAutoMigrate != "" {
		parsedAutoMigrate, err := strconv.ParseBool(envAutoMigrate)
		if err != nil {
			return Config{}, fmt.Errorf("POISED_DATABASE_AUTO_MIGRATE must be a boolean: %w", err)
		}
		config.Database.AutoMigrate = parsedAutoMigrate
	}
	if envMaxConns := os.Getenv("POISED_DATABASE_MAX_CONNS"); envMaxConns != "" {
		parsedMaxConns, err := strconv.ParseInt(envMaxConns, 10, 32)
		if err != nil {
			return Config{}, fmt.Errorf("POISED_DATABASE_MAX_CONNS must be a number: %w", err)
		}
		config.Database.MaxConns = int32(parsedMaxConns)
	}
	if config.Database.MaxConns == 0 {
		config.Database.MaxConns = 5
	}

	if err := validate(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

func validate(config Config) error {
	if config.Database.URL == "" {
		return fmt.Errorf("database url is required")
	}
	if config.Database.MaxConns < 0 {
		return fmt.Errorf("database max_conns must be greater than or equal to 0")
	}

	seenJobs := make(map[string]struct{}, len(config.Jobs))
	for _, job := range config.Jobs {
		if job.ID == "" {
			return fmt.Errorf("job id is required")
		}
		if !jobIDPattern.MatchString(job.ID) {
			return fmt.Errorf("job %q id must be URL-safe and use only letters, numbers, dots, underscores, or hyphens", job.ID)
		}
		if job.Adapter == "" {
			return fmt.Errorf("job %q adapter is required", job.ID)
		}
		if _, exists := seenJobs[job.ID]; exists {
			return fmt.Errorf("job %q is duplicated", job.ID)
		}
		seenJobs[job.ID] = struct{}{}
		if job.Enabled {
			if err := validateWholeSecondDuration(job.Interval, "interval"); err != nil {
				return fmt.Errorf("job %q %w", job.ID, err)
			}
		} else if job.Interval != "" {
			if err := validateWholeSecondDuration(job.Interval, "interval"); err != nil {
				return fmt.Errorf("job %q %w", job.ID, err)
			}
		}
		if job.Timeout != "" {
			if err := validateWholeSecondDuration(job.Timeout, "timeout"); err != nil {
				return fmt.Errorf("job %q %w", job.ID, err)
			}
		}
	}

	return nil
}

func validateWholeSecondDuration(raw string, field string) error {
	if raw == "" {
		return fmt.Errorf("%s is required", field)
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", field, err)
	}
	if duration <= 0 {
		return fmt.Errorf("%s must be positive", field)
	}
	if duration%time.Second != 0 {
		return fmt.Errorf("%s must be a whole number of seconds", field)
	}
	return nil
}
