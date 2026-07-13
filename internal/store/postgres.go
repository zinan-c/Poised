package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zinan-c/Poised/internal/core"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

type MonitorTask struct {
	ID              string          `json:"id"`
	Key             string          `json:"key"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Enabled         bool            `json:"enabled"`
	Status          string          `json:"status"`
	IntervalSeconds int             `json:"interval_seconds"`
	TimeoutSeconds  int             `json:"timeout_seconds"`
	TaskConfig      json.RawMessage `json:"task_config"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type MonitorRecord struct {
	ID         string          `json:"id"`
	TaskID     string          `json:"task_id"`
	RunID      string          `json:"run_id"`
	Channel    string          `json:"channel"`
	Adapter    string          `json:"adapter_name"`
	RecordType string          `json:"record_type"`
	RecordKey  string          `json:"record_key"`
	ObservedAt time.Time       `json:"observed_at"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (store *PostgresStore) UpsertTask(ctx context.Context, job core.JobSpec) (MonitorTask, error) {
	if store == nil || store.pool == nil {
		return MonitorTask{}, fmt.Errorf("postgres store is not open")
	}

	intervalSeconds, err := durationSeconds(job.Interval, time.Minute)
	if err != nil {
		return MonitorTask{}, fmt.Errorf("parse job %q interval: %w", job.ID, err)
	}
	timeoutSeconds, err := durationSeconds(job.Timeout, 30*time.Second)
	if err != nil {
		return MonitorTask{}, fmt.Errorf("parse job %q timeout: %w", job.ID, err)
	}

	status := "active"
	if !job.Enabled {
		status = "paused"
	}

	taskConfig, err := json.Marshal(map[string]any{
		"adapter": job.Adapter,
		"payload": json.RawMessage(job.Payload),
	})
	if err != nil {
		return MonitorTask{}, fmt.Errorf("marshal task config: %w", err)
	}

	return store.upsertTask(ctx, upsertTaskInput{
		Key:             job.ID,
		Name:            job.Name,
		Enabled:         job.Enabled,
		Status:          status,
		IntervalSeconds: intervalSeconds,
		TimeoutSeconds:  timeoutSeconds,
		TaskConfig:      taskConfig,
	})
}

func (store *PostgresStore) ListTasks(ctx context.Context, limit int) ([]MonitorTask, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("postgres store is not open")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := store.pool.Query(ctx, `
SELECT id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
FROM monitor_tasks
ORDER BY updated_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list monitor tasks: %w", err)
	}
	defer rows.Close()

	tasks, err := pgx.CollectRows(rows, scanMonitorTask)
	if err != nil {
		return nil, fmt.Errorf("scan monitor tasks: %w", err)
	}
	return tasks, nil
}

func (store *PostgresStore) SaveRun(ctx context.Context, run core.JobRun) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("postgres store is not open")
	}

	taskID, err := store.ensureTaskID(ctx, run.JobID)
	if err != nil {
		return err
	}

	resultJSON, err := json.Marshal(run.Result)
	if err != nil {
		return fmt.Errorf("marshal run result: %w", err)
	}
	summaryJSON, err := json.Marshal(map[string]any{
		"text":   run.Summary,
		"result": json.RawMessage(resultJSON),
	})
	if err != nil {
		return fmt.Errorf("marshal run summary: %w", err)
	}

	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin save run transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
INSERT INTO monitor_runs (
    id, task_id, adapter_name, status, started_at, finished_at, duration_ms, error_message, adapter_payload, summary
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
ON CONFLICT (id) DO UPDATE SET
    task_id = EXCLUDED.task_id,
    adapter_name = EXCLUDED.adapter_name,
    status = EXCLUDED.status,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at,
    duration_ms = EXCLUDED.duration_ms,
    error_message = EXCLUDED.error_message,
    adapter_payload = EXCLUDED.adapter_payload,
    summary = EXCLUDED.summary
`, run.ID, taskID, run.Adapter, string(run.Status), run.StartedAt, run.FinishedAt, run.DurationMillis, run.Error, resultJSON, summaryJSON); err != nil {
		return fmt.Errorf("insert monitor run: %w", err)
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO monitor_records (
    task_id, run_id, channel, adapter_name, record_type, record_key, observed_at, payload
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (run_id, record_type, record_key) WHERE run_id IS NOT NULL DO UPDATE SET
    task_id = EXCLUDED.task_id,
    channel = EXCLUDED.channel,
    adapter_name = EXCLUDED.adapter_name,
    observed_at = EXCLUDED.observed_at,
    payload = EXCLUDED.payload
`, taskID, run.ID, run.Adapter, run.Adapter, "run_result", run.ID, run.FinishedAt, resultJSON); err != nil {
		return fmt.Errorf("insert monitor record: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit save run transaction: %w", err)
	}
	return nil
}

func (store *PostgresStore) ListRuns(ctx context.Context, limit int) ([]core.JobRun, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("postgres store is not open")
	}
	if limit <= 0 {
		limit = 50
	}

	rows, err := store.pool.Query(ctx, `
SELECT
    runs.id::text,
    COALESCE(tasks.key, ''),
    runs.adapter_name,
    runs.status,
    runs.started_at,
    COALESCE(runs.finished_at, runs.started_at),
    runs.duration_ms,
    runs.error_message,
    runs.summary,
    runs.adapter_payload
FROM monitor_runs runs
LEFT JOIN monitor_tasks tasks ON tasks.id = runs.task_id
ORDER BY runs.started_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list monitor runs: %w", err)
	}
	defer rows.Close()

	runs := make([]core.JobRun, 0)
	for rows.Next() {
		run, err := scanJobRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitor runs: %w", err)
	}
	return runs, nil
}

func (store *PostgresStore) ListRecords(ctx context.Context, limit int) ([]MonitorRecord, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("postgres store is not open")
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := store.pool.Query(ctx, `
SELECT id::text, COALESCE(task_id::text, ''), COALESCE(run_id::text, ''), channel, adapter_name, record_type, record_key, observed_at, payload, created_at
FROM monitor_records
ORDER BY observed_at DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, fmt.Errorf("list monitor records: %w", err)
	}
	defer rows.Close()

	records, err := pgx.CollectRows(rows, scanMonitorRecord)
	if err != nil {
		return nil, fmt.Errorf("scan monitor records: %w", err)
	}
	return records, nil
}

type upsertTaskInput struct {
	Key             string
	Name            string
	Enabled         bool
	Status          string
	IntervalSeconds int
	TimeoutSeconds  int
	TaskConfig      []byte
}

func (store *PostgresStore) upsertTask(ctx context.Context, input upsertTaskInput) (MonitorTask, error) {
	var task MonitorTask
	if err := store.pool.QueryRow(ctx, `
INSERT INTO monitor_tasks (
    key, name, enabled, status, interval_seconds, timeout_seconds, task_config
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
ON CONFLICT (key) DO UPDATE SET
    name = EXCLUDED.name,
    enabled = EXCLUDED.enabled,
    status = EXCLUDED.status,
    interval_seconds = EXCLUDED.interval_seconds,
    timeout_seconds = EXCLUDED.timeout_seconds,
    task_config = EXCLUDED.task_config,
    updated_at = now()
RETURNING id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
`, input.Key, input.Name, input.Enabled, input.Status, input.IntervalSeconds, input.TimeoutSeconds, input.TaskConfig).Scan(
		&task.ID,
		&task.Key,
		&task.Name,
		&task.Description,
		&task.Enabled,
		&task.Status,
		&task.IntervalSeconds,
		&task.TimeoutSeconds,
		&task.TaskConfig,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return MonitorTask{}, fmt.Errorf("upsert monitor task %q: %w", input.Key, err)
	}
	return task, nil
}

func (store *PostgresStore) ensureTaskID(ctx context.Context, key string) (string, error) {
	var taskID string
	err := store.pool.QueryRow(ctx, "SELECT id::text FROM monitor_tasks WHERE key = $1", key).Scan(&taskID)
	if err == nil {
		return taskID, nil
	}
	if err != pgx.ErrNoRows {
		return "", fmt.Errorf("lookup monitor task %q: %w", key, err)
	}

	task, err := store.upsertTask(ctx, upsertTaskInput{
		Key:             key,
		Name:            key,
		Enabled:         true,
		Status:          "active",
		IntervalSeconds: 60,
		TimeoutSeconds:  30,
		TaskConfig:      []byte(`{}`),
	})
	if err != nil {
		return "", err
	}
	return task.ID, nil
}

func scanMonitorTask(row pgx.CollectableRow) (MonitorTask, error) {
	var task MonitorTask
	err := row.Scan(
		&task.ID,
		&task.Key,
		&task.Name,
		&task.Description,
		&task.Enabled,
		&task.Status,
		&task.IntervalSeconds,
		&task.TimeoutSeconds,
		&task.TaskConfig,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	return task, err
}

func scanMonitorRecord(row pgx.CollectableRow) (MonitorRecord, error) {
	var record MonitorRecord
	err := row.Scan(
		&record.ID,
		&record.TaskID,
		&record.RunID,
		&record.Channel,
		&record.Adapter,
		&record.RecordType,
		&record.RecordKey,
		&record.ObservedAt,
		&record.Payload,
		&record.CreatedAt,
	)
	return record, err
}

func scanJobRun(rows pgx.Rows) (core.JobRun, error) {
	var run core.JobRun
	var summaryJSON json.RawMessage
	var resultJSON json.RawMessage

	if err := rows.Scan(
		&run.ID,
		&run.JobID,
		&run.Adapter,
		&run.Status,
		&run.StartedAt,
		&run.FinishedAt,
		&run.DurationMillis,
		&run.Error,
		&summaryJSON,
		&resultJSON,
	); err != nil {
		return core.JobRun{}, fmt.Errorf("scan monitor run: %w", err)
	}

	if len(resultJSON) > 0 {
		if err := json.Unmarshal(resultJSON, &run.Result); err != nil {
			return core.JobRun{}, fmt.Errorf("decode run result %q: %w", run.ID, err)
		}
	}
	if run.Summary == "" {
		run.Summary = run.Result.Summary
	}
	if run.Result.Status == "" {
		run.Result.Status = run.Status
	}
	return run, nil
}

func durationSeconds(raw string, fallback time.Duration) (int, error) {
	if raw == "" {
		return int(fallback.Seconds()), nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return int(duration.Seconds()), nil
}
