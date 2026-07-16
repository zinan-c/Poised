# Poised Code Review

Review date: 2026-07-16

## Scope

This review covers the complete repository, including:

- Go commands, configuration, scheduler, runner, API, database, and stores
- Adapter registry, HTTP check adapter, and airfare adapters
- PostgreSQL schema and persistence behavior
- Web console assets
- Docker and local/integration scripts
- Existing tests and build configuration

The review was read-only with respect to application code. No production behavior
was changed.

## Summary

The project has a clear, compact structure and the existing unit tests, `go vet`,
race detector, and builds pass. The main risks are concentrated around cancellation
and persistence, configuration inconsistencies, externally exposed API operations,
and unbounded resource consumption.

The highest-priority issue is that canceled runs can be lost instead of persisted.
There is also a configuration path where an enabled job is stored as active but is
never scheduled.

## Findings

### P1: Canceled or interrupted runs may not be persisted

Location: [`internal/runner/runner.go`](../internal/runner/runner.go), `RunJob` and
`finish`.

`RunJob` passes the caller's context to `finish`, and `finish` uses it for
`SaveRun`. If that context has already been canceled, the store immediately fails
with `context canceled`.

This can happen when:

- the service receives `SIGTERM` while a scheduled run is active;
- an HTTP client disconnects during a manual run;
- an upstream request is canceled through the parent context.

The returned/logged run may appear canceled or failed, but the corresponding
database run and record are missing. These are precisely the executions that
usually require an audit trail.

Recommended change:

- Save the final run with a separate, short-lived persistence context, such as a
  background context with a 5–10 second timeout.
- Preserve the cancellation reason and classify parent cancellation consistently
  as `canceled`.
- Add tests using a canceled parent context and a store that records the context
  passed to `SaveRun`.

### P1: An enabled job with an empty interval is active in PostgreSQL but never scheduled

Locations:

- [`internal/store/postgres.go`](../internal/store/postgres.go), `UpsertTask`
- [`internal/scheduler/scheduler.go`](../internal/scheduler/scheduler.go), `Start`

The persistence layer treats an empty interval as one minute. The scheduler instead
calls `time.ParseDuration("")`, logs an error, and skips the job.

The resulting state is misleading:

- `monitor_tasks` reports the job as enabled and active with a 60-second interval;
- the scheduler never executes it.

This is a silent monitoring failure.

Recommended change:

- Define the default interval once and apply it before both persistence and
  scheduling; or
- require every enabled job to provide a valid positive interval during
  configuration validation.

### P1: API access can expose secrets and trigger unlimited work

Locations:

- [`internal/api/server.go`](../internal/api/server.go), `handleJobs` and
  `handleJobRun`
- [`docker-compose.yml`](../docker-compose.yml)

The API has no authentication, authorization, rate limiting, or per-job concurrency
control. Docker config binds the service to `0.0.0.0`.

Potential impact:

- `/v1/jobs` returns full job payloads. The HTTP-check adapter supports arbitrary
  headers, so payloads may include bearer tokens or other credentials.
- Any reachable client can trigger configured jobs.
- Repeated POST requests can run the same job concurrently, including alongside
  its scheduled execution.
- Expensive external requests and database writes can be amplified into a
  denial-of-service condition.

Recommended change:

- Return a sanitized job representation that excludes payload secrets.
- Require authentication before exposing the service outside localhost.
- Add request rate limits and a configurable per-job/global concurrency limit.
- Consider a CSRF defense if the console uses cookie-based authentication.

### P2: Airfare response bodies are read without a size limit

Location: [`internal/adapters/airfare/httpjson.go`](../internal/adapters/airfare/httpjson.go),
`JSONClient.do`.

The client uses `io.ReadAll(response.Body)`. A broken or malicious upstream can
return an arbitrarily large response and exhaust process memory.

Recommended change:

- Read through `io.LimitReader`.
- Define a maximum response size appropriate for the supported APIs.
- Return a clear error when the response exceeds the limit.
- Optionally validate `Content-Type` before decoding JSON.

The CLI also uses an unbounded `io.ReadAll`, but its impact is limited to the CLI
process.

### P2: Collection query limits have no upper bound

Location: [`internal/api/server.go`](../internal/api/server.go), `parseLimit`.

The API accepts any integer and passes large positive values directly to PostgreSQL.
For example, `?limit=100000000` can cause a large query, JSON allocation, and
response.

Negative and zero values are handled inconsistently: they are accepted by the API
and silently replaced by store defaults.

Recommended change:

- Reject values below 1.
- Clamp or reject values above an explicit maximum, such as 500 or 1,000.
- Apply the same rule at the API and store boundaries.

### P2: Sub-second durations are truncated

Location: [`internal/store/postgres.go`](../internal/store/postgres.go),
`durationSeconds`.

Durations are converted using `int(duration.Seconds())`.

Consequences:

- `500ms` becomes `0`, violating the positive PostgreSQL constraint.
- `1500ms` is stored and displayed as `1s`, while the scheduler executes it as
  `1.5s`.
- API/database state can disagree with actual runtime behavior.

Recommended change:

- Explicitly support only whole-second values and reject anything smaller or
  fractional; or
- store durations in milliseconds and rename the schema fields accordingly.

All duration values should also be validated as positive before opening the
database.

### P2: China Eastern empty results are reported as success

Location: [`internal/adapters/airfare/ceair/ceair.go`](../internal/adapters/airfare/ceair/ceair.go),
`Adapter.Run`.

The China Southern and Spring Airlines adapters return `failed` when no normalized
observations are produced. The China Eastern adapter always returns `success` after
a nominal API response, even when the observation list is empty.

An empty result can mean no inventory, but it can also indicate:

- a response schema change;
- an incomplete response;
- WAF or bot mitigation behavior;
- incorrect price-to-flight mapping.

Recommended change:

- Apply a consistent empty-result policy across airfare adapters.
- If “no inventory” is a valid outcome, represent it explicitly rather than using
  a generic success.
- Add a China Eastern empty-result test.

### P2: PostgreSQL integration startup is racy

Location: [`scripts/integration_postgres.sh`](../scripts/integration_postgres.sh).

The script starts PostgreSQL with `docker compose up -d postgres` and immediately
starts the Go service. It does not wait for the container health check or
`pg_isready`.

On a cold Docker start, the application can attempt its initial database check
before PostgreSQL is ready and exit.

Recommended change:

- Use `docker compose up --wait -d postgres` where supported; or
- poll `pg_isready` before starting the application.

The script also exposes a fixed host port, which can conflict with an existing
PostgreSQL instance.

### P3: The manual run endpoint returns 202 after completing work synchronously

Location: [`internal/api/server.go`](../internal/api/server.go), `handleJobRun`.

The handler waits for the full adapter execution and database save, then returns
`202 Accepted`. This status normally means the work has only been accepted for
later processing.

Recommended change:

- Return `200 OK` if execution remains synchronous; or
- enqueue the run and immediately return `202` with a run ID and status endpoint.

## Additional Refinements

### Configuration

- Use `json.Decoder.DisallowUnknownFields` so misspelled fields do not silently
  fall back to defaults.
- Validate positive intervals and timeouts during configuration loading.
- Validate passenger counts and required return dates for round-trip searches.
- Validate that configured adapters exist before syncing tasks.
- Restrict job IDs to a URL-safe format. IDs containing `/` pass current config
  validation but cannot be addressed reliably through the manual-run route.
- Validate adapter base URLs as HTTP or HTTPS URLs rather than accepting any
  scheme.

### Scheduler and runner

- Add panic recovery around adapter execution so a faulty adapter cannot terminate
  the whole process.
- Define and enforce overlap behavior for scheduled and manual runs.
- Consider jitter or backoff for large groups of jobs to avoid synchronized bursts.
- Bound shutdown waiting. `Scheduler.Wait` can block forever if a future adapter
  ignores context cancellation.
- Add a distinct timeout status if the database schema is intended to distinguish
  timeouts from ordinary failures.

### Database and stores

- Reconcile jobs removed from configuration. Currently, old rows remain in
  `monitor_tasks` and may continue to appear active.
- Clarify the meaning of `adapter_payload`: it currently stores the adapter result,
  not the adapter input payload.
- `summary` stores a second copy of the result, while `ListRuns` ignores the saved
  summary JSON and derives the text from `adapter_payload`.
- Add schema-versioned migrations. `CREATE TABLE IF NOT EXISTS` verifies table
  presence but does not detect incompatible columns, constraints, or indexes.
- Consider using `BIGINT` for `duration_ms`; the Go model uses `int64` while the
  PostgreSQL column is `INTEGER`.
- Use per-operation startup timeouts when syncing many jobs instead of one shared
  10-second context for the whole loop.

### HTTP and API

- Configure an `IdleTimeout` and appropriate read/write safeguards on the HTTP
  server.
- Avoid returning raw database error strings to clients.
- Add response status/duration logging rather than logging only request method and
  path.
- Decide whether disabled jobs may be run manually and enforce/document the
  decision.
- Add a maximum response size to `poisedctl`.

### Web console

- `refreshAll` uses one `Promise.all`; one failed endpoint causes the entire page
  to report “API is unreachable” even if health and most endpoints work.
- Render each panel independently or use `Promise.allSettled`.
- Avoid returning full secret-bearing job payloads solely to populate the job
  selector.
- Embed static files with Go's `embed` package. Current relative filesystem paths
  fail when the binary is launched from a working directory that does not contain
  `web/`.

### Scripts and deployment

- `run_local.sh` always checks and provisions PostgreSQL on `127.0.0.1:5432`, even
  when `POISED_DATABASE_URL` points to a different server.
- Avoid automatically resetting the local `poised` role password on every start.
- Consider a Docker health check for the application service.
- Keep the runtime Go version aligned with the version declared in `go.mod`, unless
  the difference is intentional and tested.

## Test Gaps

The existing tests cover registry behavior, runner persistence, HTTP-check
responses, and representative airfare adapter responses. Important untested areas
include:

- configuration defaults, invalid durations, and unknown fields;
- scheduler startup, cancellation, and overlap behavior;
- API routing, limits, error responses, and manual runs;
- PostgreSQL store serialization and transaction behavior;
- canceled-context persistence;
- empty-result consistency across adapters;
- oversized upstream responses;
- static asset serving outside the repository working directory;
- startup and shutdown behavior.

Recommended priority tests:

1. A canceled run is still saved with `canceled` status.
2. An enabled job with an empty or non-positive interval is rejected or scheduled
   with the documented default.
3. API limits reject negative and excessive values.
4. Airfare clients reject oversized responses.
5. China Eastern empty observations follow the chosen policy.
6. Manual runs obey concurrency and disabled-job rules.
7. PostgreSQL integration waits for database readiness.

## Verification Performed

The following checks completed successfully:

```text
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/poised ./cmd/poisedctl
node --check web/assets/app.js
```

`shellcheck` was not available in the environment.


## Remediation Comments

Date: 2026-07-16

### Findings

- P1 canceled/interrupted runs: Implemented. Runner persistence now uses a separate background timeout context, parent cancellation is normalized to `canceled`, and tests cover saving with a canceled parent context.
- P1 empty interval mismatch: Implemented. Configuration validation now rejects enabled jobs without a positive whole-second interval, and PostgreSQL duration conversion rejects fractional or non-positive durations.
- P1 API exposure and unlimited work: Partially implemented. `/v1/jobs` now returns a sanitized job view without payloads, request logs include status and duration, and per-job execution is serialized in the runner. Authentication/rate limiting remain a follow-up before binding the service outside trusted networks.
- P2 airfare unbounded body reads: Implemented for shared airfare HTTP client with a 10 MiB response cap and an oversized-response test. `poisedctl` also now caps response reads.
- P2 collection query limits: Implemented. API limits must be between 1 and 500, and PostgreSQL store methods clamp to the same maximum as a second boundary.
- P2 sub-second duration truncation: Implemented by policy. Durations must be positive whole-second values; fractional durations are rejected during config loading and store conversion.
- P2 China Eastern empty result success: Implemented. CEAir now returns failed for empty observations, matching CSAir and SpringAir behavior, with a dedicated test.
- P2 PostgreSQL integration startup race: Implemented for the Docker integration script by polling `pg_isready` inside the postgres service before starting the app. Host-port configurability remains a follow-up because the compose file still exposes a fixed Postgres port.
- P3 synchronous manual run returns 202: Implemented. Manual run now returns `200 OK` because work is completed synchronously.

### Additional Refinements

- Configuration unknown fields: Implemented with `json.Decoder.DisallowUnknownFields` and a test.
- Positive intervals/timeouts: Implemented for configured durations, including a fractional-duration test.
- Passenger counts and round-trip rules: Not implemented yet; should be added inside each airfare payload validator because the rules are adapter-specific.
- Configured adapters exist: Implemented in startup after adapter registration and before DB sync.
- URL-safe job IDs: Implemented in config validation.
- Adapter base URL scheme validation: Already partially present in the shared airfare `JSONClient`; adapter-specific payload validators can still add stricter checks later.
- Adapter panic recovery: Implemented in runner with a failed persisted run and a test.
- Overlap behavior: Partially implemented. Runner serializes executions per job, covering scheduled/manual overlap. Global concurrency limits remain a follow-up.
- Jitter/backoff: Not implemented; defer until job volume makes synchronized bursts a real operational issue.
- Bounded shutdown waiting: Not implemented; scheduler still waits for adapters to honor cancellation. Add a bounded wait wrapper if adapters become less trusted.
- Distinct timeout status: Not implemented; current model still maps timeout to failed. Add `timeout` to `core.RunStatus` in a separate schema/model migration.
- Removed jobs reconciliation: Not implemented; add archive-on-sync for tasks missing from config in a future repository pass.
- `adapter_payload` naming: Not implemented; requires schema migration because the column currently stores result payloads.
- `summary` duplicate result storage: Not implemented; should be cleaned with the `adapter_payload`/result schema migration.
- Schema-versioned migrations: Not implemented; recommended before production use.
- `duration_ms` BIGINT: Not implemented; should be included in the first schema migration set.
- Per-operation startup sync timeouts: Not implemented; current sync still uses one 10-second startup context.
- HTTP server timeouts: Implemented with read, write, idle, and read-header timeouts.
- Raw DB errors to clients: Partially implemented for task/run/record listing. More handlers can be normalized as they are added.
- Response status/duration logging: Implemented.
- Disabled manual runs: Decision remains unchanged; manual run is still allowed for configured jobs regardless of enabled flag. Document or restrict when permissions are introduced.
- `poisedctl` max response size: Implemented.
- Web console `Promise.all` fragility: Implemented with `Promise.allSettled` and per-panel errors.
- Web console secret-bearing jobs: Implemented on the API side by sanitizing `/v1/jobs` responses.
- Static asset relative paths: Implemented with Go `embed`, so the binary no longer depends on the process working directory containing `web/`.
- `run_local.sh` external DB URLs: Implemented. Local provisioning only runs for the default local database URL; custom URLs are only checked for readiness.
- `run_local.sh` password reset: Implemented. The script sets the password only when it creates the local role.
- Docker app health check: Not implemented yet; add a `/healthz` healthcheck to the app service in `docker-compose.yml` when Docker-based deployment becomes active.
- Go version alignment: Not implemented yet; Dockerfile still uses the current base image. Align it with `go.mod` or intentionally bump `go.mod` in a separate toolchain pass.

### Test Gap Updates

- Canceled-context persistence: Covered by a runner test.
- Invalid durations and unknown fields: Covered by config tests.
- API limits: Covered by API tests for invalid lower and upper bounds.
- Oversized upstream responses: Covered by a shared airfare HTTP client test.
- CEAir empty observations: Covered by a CEAir adapter test.
- Manual run concurrency and disabled-job rules: Per-job concurrency implemented indirectly; explicit API tests still recommended.
- PostgreSQL integration readiness: Docker integration script now waits for readiness; local integration was previously verified against Homebrew PostgreSQL.

### Remediation Verification

The remediation pass completed the following checks successfully:

```text
node --check web/assets/app.js
GOCACHE=/tmp/poised-go-cache go test ./...
GOCACHE=/tmp/poised-go-cache go vet ./...
GOCACHE=/tmp/poised-go-cache go build ./cmd/poised ./cmd/poisedctl
```

`go test ./...` was run outside the filesystem/network sandbox because several
existing tests use `httptest` and need to bind local loopback ports.
