#!/usr/bin/env bash
set -euo pipefail

project_name="${POISED_INTEGRATION_PROJECT:-poised_it}"
http_addr="${POISED_INTEGRATION_HTTP_ADDR:-127.0.0.1:18080}"
database_url="${POISED_DATABASE_URL:-postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable}"
base_url="http://${http_addr}"
log_file="${TMPDIR:-/tmp}/poised-integration.log"

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for PostgreSQL integration." >&2
  exit 127
fi

cleanup() {
  if [[ -n "${poised_pid:-}" ]]; then
    kill "${poised_pid}" >/dev/null 2>&1 || true
    wait "${poised_pid}" >/dev/null 2>&1 || true
  fi
  docker compose -p "${project_name}" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

docker compose -p "${project_name}" up -d postgres

export GOCACHE="${GOCACHE:-/tmp/poised-go-cache}"
export POISED_HTTP_ADDR="${http_addr}"
export POISED_DATABASE_URL="${database_url}"
export POISED_DATABASE_AUTO_MIGRATE=true
export POISED_DATABASE_MAX_CONNS=5

go run ./cmd/poised -config configs/poised.example.json >"${log_file}" 2>&1 &
poised_pid=$!

for _ in {1..60}; do
  if curl -fsS "${base_url}/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "${poised_pid}" >/dev/null 2>&1; then
    cat "${log_file}"
    exit 1
  fi
  sleep 1
done

curl -fsS "${base_url}/healthz" >/dev/null
curl -fsS "${base_url}/v1/tasks?limit=10" >/dev/null
curl -fsS -X POST "${base_url}/v1/jobs/example-echo/runs" >/dev/null
curl -fsS "${base_url}/v1/runs?limit=10" >/dev/null
curl -fsS "${base_url}/v1/records?limit=10" >/dev/null

echo "PostgreSQL integration passed."
