package database

var RequiredTables = []string{
	"monitor_tasks",
	"monitor_task_channels",
	"monitor_runs",
	"monitor_records",
	"monitor_alert_rules",
	"monitor_alert_events",
}

const SchemaSQL = `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS monitor_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    key TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT true,
    status TEXT NOT NULL DEFAULT 'active',
    interval_seconds INTEGER NOT NULL,
    timeout_seconds INTEGER NOT NULL DEFAULT 30,
    task_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT monitor_tasks_status_check CHECK (status IN ('active', 'paused', 'archived')),
    CONSTRAINT monitor_tasks_interval_check CHECK (interval_seconds > 0),
    CONSTRAINT monitor_tasks_timeout_check CHECK (timeout_seconds > 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS monitor_tasks_key_uidx ON monitor_tasks (key);

CREATE TABLE IF NOT EXISTS monitor_task_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES monitor_tasks(id) ON DELETE CASCADE,
    channel TEXT NOT NULL,
    adapter_name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    adapter_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, channel)
);

CREATE TABLE IF NOT EXISTS monitor_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID REFERENCES monitor_tasks(id) ON DELETE SET NULL,
    channel_id UUID REFERENCES monitor_task_channels(id) ON DELETE SET NULL,
    adapter_name TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    adapter_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    summary JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT monitor_runs_status_check CHECK (status IN ('success', 'failed', 'canceled', 'timeout', 'skipped'))
);

CREATE TABLE IF NOT EXISTS monitor_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID REFERENCES monitor_tasks(id) ON DELETE SET NULL,
    run_id UUID REFERENCES monitor_runs(id) ON DELETE SET NULL,
    channel TEXT NOT NULL,
    adapter_name TEXT NOT NULL,
    record_type TEXT NOT NULL,
    record_key TEXT NOT NULL DEFAULT '',
    observed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS monitor_alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL REFERENCES monitor_tasks(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    rule_type TEXT NOT NULL,
    rule_config JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS monitor_alert_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_rule_id UUID REFERENCES monitor_alert_rules(id) ON DELETE SET NULL,
    task_id UUID REFERENCES monitor_tasks(id) ON DELETE SET NULL,
    record_id UUID REFERENCES monitor_records(id) ON DELETE SET NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    status TEXT NOT NULL DEFAULT 'open',
    message TEXT NOT NULL DEFAULT '',
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT monitor_alert_events_severity_check CHECK (severity IN ('info', 'warning', 'critical')),
    CONSTRAINT monitor_alert_events_status_check CHECK (status IN ('open', 'acknowledged', 'resolved'))
);

CREATE INDEX IF NOT EXISTS monitor_tasks_enabled_status_idx ON monitor_tasks (enabled, status);
CREATE INDEX IF NOT EXISTS monitor_tasks_key_idx ON monitor_tasks (key);
CREATE INDEX IF NOT EXISTS monitor_tasks_task_config_gin_idx ON monitor_tasks USING GIN (task_config);

CREATE INDEX IF NOT EXISTS monitor_task_channels_task_id_idx ON monitor_task_channels (task_id);
CREATE INDEX IF NOT EXISTS monitor_task_channels_enabled_idx ON monitor_task_channels (enabled);
CREATE INDEX IF NOT EXISTS monitor_task_channels_adapter_config_gin_idx ON monitor_task_channels USING GIN (adapter_config);

CREATE INDEX IF NOT EXISTS monitor_runs_task_started_idx ON monitor_runs (task_id, started_at DESC);
CREATE INDEX IF NOT EXISTS monitor_runs_channel_started_idx ON monitor_runs (channel_id, started_at DESC);
CREATE INDEX IF NOT EXISTS monitor_runs_status_idx ON monitor_runs (status);
CREATE INDEX IF NOT EXISTS monitor_runs_adapter_payload_gin_idx ON monitor_runs USING GIN (adapter_payload);

CREATE INDEX IF NOT EXISTS monitor_records_task_observed_idx ON monitor_records (task_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS monitor_records_run_id_idx ON monitor_records (run_id);
CREATE UNIQUE INDEX IF NOT EXISTS monitor_records_run_type_key_uidx ON monitor_records (run_id, record_type, record_key) WHERE run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS monitor_records_record_type_idx ON monitor_records (record_type);
CREATE INDEX IF NOT EXISTS monitor_records_record_key_idx ON monitor_records (record_key);
CREATE INDEX IF NOT EXISTS monitor_records_payload_gin_idx ON monitor_records USING GIN (payload);

CREATE INDEX IF NOT EXISTS monitor_alert_rules_task_id_idx ON monitor_alert_rules (task_id);
CREATE INDEX IF NOT EXISTS monitor_alert_events_task_triggered_idx ON monitor_alert_events (task_id, triggered_at DESC);
CREATE INDEX IF NOT EXISTS monitor_alert_events_status_idx ON monitor_alert_events (status);
`
