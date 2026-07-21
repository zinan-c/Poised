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

const maxStoreLimit = 500

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

type MonitorTaskChannel struct {
	ID            string          `json:"id"`
	TaskID        string          `json:"task_id"`
	Channel       string          `json:"channel"`
	Adapter       string          `json:"adapter_name"`
	Enabled       bool            `json:"enabled"`
	AdapterConfig json.RawMessage `json:"adapter_config"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

type TaskInput struct {
	Key             string          `json:"key"`
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	Enabled         bool            `json:"enabled"`
	Status          string          `json:"status"`
	IntervalSeconds int             `json:"interval_seconds"`
	TimeoutSeconds  int             `json:"timeout_seconds"`
	TaskConfig      json.RawMessage `json:"task_config"`
}

type ChannelInput struct {
	Channel       string          `json:"channel"`
	Adapter       string          `json:"adapter_name"`
	Enabled       bool            `json:"enabled"`
	AdapterConfig json.RawMessage `json:"adapter_config"`
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

	task, err := store.upsertTask(ctx, upsertTaskInput{
		Key:             job.ID,
		Name:            job.Name,
		Enabled:         job.Enabled,
		Status:          status,
		IntervalSeconds: intervalSeconds,
		TimeoutSeconds:  timeoutSeconds,
		TaskConfig:      taskConfig,
	})
	if err != nil {
		return MonitorTask{}, err
	}

	_, err = store.upsertChannel(ctx, task.ID, ChannelInput{
		Channel:       firstNonEmpty(job.Channel, job.Adapter),
		Adapter:       job.Adapter,
		Enabled:       job.Enabled,
		AdapterConfig: json.RawMessage(job.Payload),
	})
	if err != nil {
		return MonitorTask{}, err
	}

	return task, nil
}

func (store *PostgresStore) CreateTask(ctx context.Context, input TaskInput) (MonitorTask, error) {
	if err := validateTaskInput(input); err != nil {
		return MonitorTask{}, err
	}
	return store.upsertTask(ctx, upsertTaskInput{
		Key:             input.Key,
		Name:            input.Name,
		Description:     input.Description,
		Enabled:         input.Enabled,
		Status:          input.Status,
		IntervalSeconds: input.IntervalSeconds,
		TimeoutSeconds:  input.TimeoutSeconds,
		TaskConfig:      input.TaskConfig,
	})
}

func (store *PostgresStore) GetTask(ctx context.Context, key string) (MonitorTask, error) {
	var task MonitorTask
	if err := store.pool.QueryRow(ctx, `
SELECT id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
FROM monitor_tasks
WHERE key = $1
`, key).Scan(
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
		return MonitorTask{}, fmt.Errorf("get monitor task %q: %w", key, err)
	}
	return task, nil
}

func (store *PostgresStore) ListTasks(ctx context.Context, limit int) ([]MonitorTask, error) {
	if store == nil || store.pool == nil {
		return nil, fmt.Errorf("postgres store is not open")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > maxStoreLimit {
		limit = maxStoreLimit
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

func (store *PostgresStore) UpdateTask(ctx context.Context, key string, input TaskInput) (MonitorTask, error) {
	if input.Key == "" {
		input.Key = key
	}
	if input.Key != key {
		return MonitorTask{}, fmt.Errorf("task key cannot be changed")
	}
	if err := validateTaskInput(input); err != nil {
		return MonitorTask{}, err
	}

	var task MonitorTask
	if err := store.pool.QueryRow(ctx, `
UPDATE monitor_tasks
SET name = $2,
    description = $3,
    enabled = $4,
    status = $5,
    interval_seconds = $6,
    timeout_seconds = $7,
    task_config = $8,
    updated_at = now()
WHERE key = $1
RETURNING id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
`, key, input.Name, input.Description, input.Enabled, input.Status, input.IntervalSeconds, input.TimeoutSeconds, normalizedJSON(input.TaskConfig)).Scan(
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
		return MonitorTask{}, fmt.Errorf("update monitor task %q: %w", key, err)
	}
	return task, nil
}

func (store *PostgresStore) SetTaskStatus(ctx context.Context, key string, status string) (MonitorTask, error) {
	enabled := status == "active"
	var task MonitorTask
	if err := store.pool.QueryRow(ctx, `
UPDATE monitor_tasks
SET status = $2, enabled = $3, updated_at = now()
WHERE key = $1
RETURNING id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
`, key, status, enabled).Scan(
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
		return MonitorTask{}, fmt.Errorf("set monitor task %q status: %w", key, err)
	}
	return task, nil
}

func (store *PostgresStore) ArchiveTask(ctx context.Context, key string) (MonitorTask, error) {
	return store.SetTaskStatus(ctx, key, "archived")
}

func (store *PostgresStore) DeleteTask(ctx context.Context, key string) error {
	commandTag, err := store.pool.Exec(ctx, "DELETE FROM monitor_tasks WHERE key = $1", key)
	if err != nil {
		return fmt.Errorf("delete monitor task %q: %w", key, err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("task %q not found", key)
	}
	return nil
}

func (store *PostgresStore) CreateChannel(ctx context.Context, taskKey string, input ChannelInput) (MonitorTaskChannel, error) {
	if err := validateChannelInput(input); err != nil {
		return MonitorTaskChannel{}, err
	}
	taskID, err := store.lookupTaskID(ctx, taskKey)
	if err != nil {
		return MonitorTaskChannel{}, err
	}
	return store.upsertChannel(ctx, taskID, input)
}

func (store *PostgresStore) ListChannels(ctx context.Context, taskKey string) ([]MonitorTaskChannel, error) {
	taskID, err := store.lookupTaskID(ctx, taskKey)
	if err != nil {
		return nil, err
	}
	rows, err := store.pool.Query(ctx, `
SELECT id::text, task_id::text, channel, adapter_name, enabled, adapter_config, created_at, updated_at
FROM monitor_task_channels
WHERE task_id = $1
ORDER BY channel ASC
`, taskID)
	if err != nil {
		return nil, fmt.Errorf("list task channels %q: %w", taskKey, err)
	}
	defer rows.Close()
	channels, err := pgx.CollectRows(rows, scanMonitorTaskChannel)
	if err != nil {
		return nil, fmt.Errorf("scan task channels %q: %w", taskKey, err)
	}
	return channels, nil
}

func (store *PostgresStore) UpdateChannel(ctx context.Context, taskKey string, channel string, input ChannelInput) (MonitorTaskChannel, error) {
	if input.Channel == "" {
		input.Channel = channel
	}
	if input.Channel != channel {
		return MonitorTaskChannel{}, fmt.Errorf("channel key cannot be changed")
	}
	if err := validateChannelInput(input); err != nil {
		return MonitorTaskChannel{}, err
	}
	taskID, err := store.lookupTaskID(ctx, taskKey)
	if err != nil {
		return MonitorTaskChannel{}, err
	}
	var taskChannel MonitorTaskChannel
	if err := store.pool.QueryRow(ctx, `
UPDATE monitor_task_channels
SET adapter_name = $3,
    enabled = $4,
    adapter_config = $5,
    updated_at = now()
WHERE task_id = $1 AND channel = $2
RETURNING id::text, task_id::text, channel, adapter_name, enabled, adapter_config, created_at, updated_at
`, taskID, channel, input.Adapter, input.Enabled, normalizedJSON(input.AdapterConfig)).Scan(
		&taskChannel.ID,
		&taskChannel.TaskID,
		&taskChannel.Channel,
		&taskChannel.Adapter,
		&taskChannel.Enabled,
		&taskChannel.AdapterConfig,
		&taskChannel.CreatedAt,
		&taskChannel.UpdatedAt,
	); err != nil {
		return MonitorTaskChannel{}, fmt.Errorf("update task channel %q/%q: %w", taskKey, channel, err)
	}
	return taskChannel, nil
}

func (store *PostgresStore) SetChannelEnabled(ctx context.Context, taskKey string, channel string, enabled bool) (MonitorTaskChannel, error) {
	taskID, err := store.lookupTaskID(ctx, taskKey)
	if err != nil {
		return MonitorTaskChannel{}, err
	}
	var taskChannel MonitorTaskChannel
	if err := store.pool.QueryRow(ctx, `
UPDATE monitor_task_channels
SET enabled = $3, updated_at = now()
WHERE task_id = $1 AND channel = $2
RETURNING id::text, task_id::text, channel, adapter_name, enabled, adapter_config, created_at, updated_at
`, taskID, channel, enabled).Scan(
		&taskChannel.ID,
		&taskChannel.TaskID,
		&taskChannel.Channel,
		&taskChannel.Adapter,
		&taskChannel.Enabled,
		&taskChannel.AdapterConfig,
		&taskChannel.CreatedAt,
		&taskChannel.UpdatedAt,
	); err != nil {
		return MonitorTaskChannel{}, fmt.Errorf("set task channel %q/%q enabled: %w", taskKey, channel, err)
	}
	return taskChannel, nil
}

func (store *PostgresStore) DeleteChannel(ctx context.Context, taskKey string, channel string) error {
	taskID, err := store.lookupTaskID(ctx, taskKey)
	if err != nil {
		return err
	}
	commandTag, err := store.pool.Exec(ctx, "DELETE FROM monitor_task_channels WHERE task_id = $1 AND channel = $2", taskID, channel)
	if err != nil {
		return fmt.Errorf("delete task channel %q/%q: %w", taskKey, channel, err)
	}
	if commandTag.RowsAffected() == 0 {
		return fmt.Errorf("channel %q/%q not found", taskKey, channel)
	}
	return nil
}

func (store *PostgresStore) ListRunnableJobs(ctx context.Context) ([]core.JobSpec, error) {
	rows, err := store.pool.Query(ctx, `
SELECT
    tasks.id::text,
    tasks.key,
    tasks.name,
    tasks.interval_seconds,
    tasks.timeout_seconds,
    channels.id::text,
    channels.channel,
    channels.adapter_name,
    channels.adapter_config
FROM monitor_tasks tasks
JOIN monitor_task_channels channels ON channels.task_id = tasks.id
WHERE tasks.enabled = true
  AND tasks.status = 'active'
  AND channels.enabled = true
ORDER BY tasks.key ASC, channels.channel ASC
`)
	if err != nil {
		return nil, fmt.Errorf("list runnable jobs: %w", err)
	}
	defer rows.Close()

	jobs := make([]core.JobSpec, 0)
	for rows.Next() {
		var taskID string
		var taskKey string
		var name string
		var intervalSeconds int
		var timeoutSeconds int
		var channelID string
		var channel string
		var adapter string
		var adapterConfig json.RawMessage
		if err := rows.Scan(&taskID, &taskKey, &name, &intervalSeconds, &timeoutSeconds, &channelID, &channel, &adapter, &adapterConfig); err != nil {
			return nil, fmt.Errorf("scan runnable job: %w", err)
		}
		jobs = append(jobs, core.JobSpec{
			ID:        taskKey,
			Name:      name,
			Adapter:   adapter,
			Enabled:   true,
			Interval:  fmt.Sprintf("%ds", intervalSeconds),
			Timeout:   fmt.Sprintf("%ds", timeoutSeconds),
			Payload:   adapterConfig,
			TaskID:    taskID,
			ChannelID: channelID,
			Channel:   channel,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runnable jobs: %w", err)
	}
	return jobs, nil
}

func (store *PostgresStore) SaveRun(ctx context.Context, run core.JobRun) error {
	if store == nil || store.pool == nil {
		return fmt.Errorf("postgres store is not open")
	}

	taskID, err := store.ensureTaskID(ctx, run.JobID)
	if err != nil {
		return err
	}
	channelID := run.ChannelID
	if channelID == "" {
		channelID, err = store.lookupChannelID(ctx, taskID, run.Channel)
		if err != nil {
			return err
		}
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
    id, task_id, channel_id, adapter_name, status, started_at, finished_at, duration_ms, error_message, adapter_payload, summary
) VALUES (
    $1, $2, NULLIF($3, '')::uuid, $4, $5, $6, $7, $8, $9, $10, $11
)
ON CONFLICT (id) DO UPDATE SET
    task_id = EXCLUDED.task_id,
    channel_id = EXCLUDED.channel_id,
    adapter_name = EXCLUDED.adapter_name,
    status = EXCLUDED.status,
    started_at = EXCLUDED.started_at,
    finished_at = EXCLUDED.finished_at,
    duration_ms = EXCLUDED.duration_ms,
    error_message = EXCLUDED.error_message,
    adapter_payload = EXCLUDED.adapter_payload,
    summary = EXCLUDED.summary
`, run.ID, taskID, channelID, run.Adapter, string(run.Status), run.StartedAt, run.FinishedAt, run.DurationMillis, run.Error, resultJSON, summaryJSON); err != nil {
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
	if limit > maxStoreLimit {
		limit = maxStoreLimit
	}

	rows, err := store.pool.Query(ctx, `
SELECT
    runs.id::text,
    COALESCE(tasks.key, ''),
    COALESCE(channels.id::text, ''),
    COALESCE(channels.channel, ''),
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
LEFT JOIN monitor_task_channels channels ON channels.id = runs.channel_id
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
	if limit > maxStoreLimit {
		limit = maxStoreLimit
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
	Description     string
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
    key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (key) DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    enabled = EXCLUDED.enabled,
    status = EXCLUDED.status,
    interval_seconds = EXCLUDED.interval_seconds,
    timeout_seconds = EXCLUDED.timeout_seconds,
    task_config = EXCLUDED.task_config,
    updated_at = now()
RETURNING id::text, key, name, description, enabled, status, interval_seconds, timeout_seconds, task_config, created_at, updated_at
`, input.Key, input.Name, input.Description, input.Enabled, input.Status, input.IntervalSeconds, input.TimeoutSeconds, normalizedJSON(input.TaskConfig)).Scan(
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

func (store *PostgresStore) lookupTaskID(ctx context.Context, key string) (string, error) {
	var taskID string
	if err := store.pool.QueryRow(ctx, "SELECT id::text FROM monitor_tasks WHERE key = $1", key).Scan(&taskID); err != nil {
		return "", fmt.Errorf("lookup monitor task %q: %w", key, err)
	}
	return taskID, nil
}

func (store *PostgresStore) lookupChannelID(ctx context.Context, taskID string, channel string) (string, error) {
	if channel == "" {
		return "", nil
	}
	var channelID string
	if err := store.pool.QueryRow(ctx, "SELECT id::text FROM monitor_task_channels WHERE task_id = $1 AND channel = $2", taskID, channel).Scan(&channelID); err != nil {
		return "", fmt.Errorf("lookup task channel %q: %w", channel, err)
	}
	return channelID, nil
}

func (store *PostgresStore) upsertChannel(ctx context.Context, taskID string, input ChannelInput) (MonitorTaskChannel, error) {
	if err := validateChannelInput(input); err != nil {
		return MonitorTaskChannel{}, err
	}
	var channel MonitorTaskChannel
	if err := store.pool.QueryRow(ctx, `
INSERT INTO monitor_task_channels (
    task_id, channel, adapter_name, enabled, adapter_config
) VALUES (
    $1, $2, $3, $4, $5
)
ON CONFLICT (task_id, channel) DO UPDATE SET
    adapter_name = EXCLUDED.adapter_name,
    enabled = EXCLUDED.enabled,
    adapter_config = EXCLUDED.adapter_config,
    updated_at = now()
RETURNING id::text, task_id::text, channel, adapter_name, enabled, adapter_config, created_at, updated_at
`, taskID, input.Channel, input.Adapter, input.Enabled, normalizedJSON(input.AdapterConfig)).Scan(
		&channel.ID,
		&channel.TaskID,
		&channel.Channel,
		&channel.Adapter,
		&channel.Enabled,
		&channel.AdapterConfig,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	); err != nil {
		return MonitorTaskChannel{}, fmt.Errorf("upsert task channel %q: %w", input.Channel, err)
	}
	return channel, nil
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

func scanMonitorTaskChannel(row pgx.CollectableRow) (MonitorTaskChannel, error) {
	var channel MonitorTaskChannel
	err := row.Scan(
		&channel.ID,
		&channel.TaskID,
		&channel.Channel,
		&channel.Adapter,
		&channel.Enabled,
		&channel.AdapterConfig,
		&channel.CreatedAt,
		&channel.UpdatedAt,
	)
	return channel, err
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
		&run.ChannelID,
		&run.Channel,
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
	if duration <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	if duration%time.Second != 0 {
		return 0, fmt.Errorf("duration must be a whole number of seconds")
	}
	return int(duration.Seconds()), nil
}

func validateTaskInput(input TaskInput) error {
	if input.Key == "" {
		return fmt.Errorf("task key is required")
	}
	if input.Name == "" {
		return fmt.Errorf("task name is required")
	}
	if input.Status == "" {
		return fmt.Errorf("task status is required")
	}
	if input.Status != "active" && input.Status != "paused" && input.Status != "archived" {
		return fmt.Errorf("task status must be active, paused, or archived")
	}
	if input.IntervalSeconds <= 0 {
		return fmt.Errorf("task interval_seconds must be positive")
	}
	if input.TimeoutSeconds <= 0 {
		return fmt.Errorf("task timeout_seconds must be positive")
	}
	if !json.Valid(normalizedJSON(input.TaskConfig)) {
		return fmt.Errorf("task_config must be valid json")
	}
	return nil
}

func validateChannelInput(input ChannelInput) error {
	if input.Channel == "" {
		return fmt.Errorf("channel is required")
	}
	if input.Adapter == "" {
		return fmt.Errorf("adapter_name is required")
	}
	if !json.Valid(normalizedJSON(input.AdapterConfig)) {
		return fmt.Errorf("adapter_config must be valid json")
	}
	return nil
}

func normalizedJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
