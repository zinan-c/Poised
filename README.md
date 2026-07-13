# Poised

Poised is a backend-only framework for recurring jobs and monitoring workflows.
The first version keeps the core small: a scheduler triggers jobs, a runner calls
registered adapters, and run results are stored through a storage interface.

## Current Stack

- Language: Go
- Runtime: single backend process
- Scheduling: in-process interval scheduler
- API: Go standard `net/http`
- Storage: PostgreSQL repositories
- Adapter model: compile-time registry for the first version

The backend uses `pgx` for PostgreSQL connection pooling and keeps adapter
execution decoupled from storage details. The next natural upgrades are a cron
scheduler and Temporal workflows.

## Run

```bash
go run ./cmd/poised -config configs/poised.example.json
```

The service listens on `127.0.0.1:8080` by default.
Set `POISED_HTTP_ADDR=0.0.0.0:8080` when running in a container.
`POISED_DATABASE_URL` is required because runtime data is always persisted to PostgreSQL.

Useful endpoints:

```bash
curl http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/v1/adapters
curl http://127.0.0.1:8080/v1/jobs
curl http://127.0.0.1:8080/v1/tasks
curl http://127.0.0.1:8080/v1/runs
curl http://127.0.0.1:8080/v1/records
curl -X POST http://127.0.0.1:8080/v1/jobs/example-echo/runs
```

The CLI can talk to the API:

```bash
go run ./cmd/poisedctl adapters
go run ./cmd/poisedctl jobs
go run ./cmd/poisedctl run example-echo
go run ./cmd/poisedctl runs
```

Or use the convenience targets:

```bash
make test
make run
make build
docker compose up --build
make integration-postgres
```

## Database

Poised initializes and checks a PostgreSQL schema at startup. `/healthz` also
checks the live database connection and required tables.

At startup, configured jobs are synced into `monitor_tasks`, job executions are
saved into `monitor_runs`, and each run emits a generic `monitor_records` row
containing the adapter result payload.

Environment variables:

```bash
POISED_DATABASE_URL=postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable
POISED_DATABASE_AUTO_MIGRATE=true
POISED_DATABASE_MAX_CONNS=5
```

Database responsibilities:

- `monitor_tasks`: user-configurable monitoring jobs with generic JSON config.
- `monitor_task_channels`: fixed channel selection and adapter-specific JSON config.
- `monitor_runs`: execution attempts for task/channel adapter runs.
- `monitor_records`: normalized generic outputs, from simple values to JSON tables.
- `monitor_alert_rules`: optional rule configs evaluated from emitted records.
- `monitor_alert_events`: alert lifecycle records tied back to rules and records.

```mermaid
erDiagram
  monitor_tasks ||--o{ monitor_task_channels : has
  monitor_tasks ||--o{ monitor_runs : creates
  monitor_task_channels ||--o{ monitor_runs : executes
  monitor_tasks ||--o{ monitor_records : emits
  monitor_runs ||--o{ monitor_records : contains
  monitor_tasks ||--o{ monitor_alert_rules : owns
  monitor_alert_rules ||--o{ monitor_alert_events : triggers
  monitor_records ||--o{ monitor_alert_events : references

  monitor_tasks {
    uuid id PK
    text key UK
    text name
    text description
    boolean enabled
    text status
    integer interval_seconds
    integer timeout_seconds
    jsonb task_config
    timestamptz created_at
    timestamptz updated_at
  }

  monitor_task_channels {
    uuid id PK
    uuid task_id FK
    text channel
    text adapter_name
    boolean enabled
    jsonb adapter_config
    timestamptz created_at
    timestamptz updated_at
  }

  monitor_runs {
    uuid id PK
    uuid task_id FK
    uuid channel_id FK
    text adapter_name
    text status
    timestamptz started_at
    timestamptz finished_at
    integer duration_ms
    text error_message
    jsonb adapter_payload
    jsonb summary
  }

  monitor_records {
    uuid id PK
    uuid task_id FK
    uuid run_id FK
    text channel
    text adapter_name
    text record_type
    text record_key
    timestamptz observed_at
    jsonb payload
    timestamptz created_at
  }

  monitor_alert_rules {
    uuid id PK
    uuid task_id FK
    text name
    boolean enabled
    text rule_type
    jsonb rule_config
    timestamptz created_at
    timestamptz updated_at
  }

  monitor_alert_events {
    uuid id PK
    uuid alert_rule_id FK
    uuid task_id FK
    uuid record_id FK
    text severity
    text status
    text message
    jsonb payload
    timestamptz triggered_at
    timestamptz resolved_at
  }
```

## Add An Adapter

Create a package under `internal/adapters/<name>` and implement:

```go
type Adapter interface {
    Name() string
    Kind() string
    Run(ctx context.Context, input core.RunInput) (core.RunResult, error)
}
```

Then register it in `cmd/poised/main.go`:

```go
registry.Register(myadapter.New())
```

Adapter payloads are configured per job in `configs/poised.example.json`.

## Target Architecture

```mermaid
flowchart LR
  Scheduler["Scheduler"] --> Runner["Runner"]
  Runner --> Registry["Adapter Registry"]
  Registry --> Adapter["Adapter"]
  Runner --> Store["Run Store"]
  Runner --> API["HTTP API"]
```

Near-term upgrades:

- Add `robfig/cron` for cron expressions.
- Add notifier adapters for Slack, Feishu, email, and webhook.
- Add Temporal when jobs need durable long-running workflows.
