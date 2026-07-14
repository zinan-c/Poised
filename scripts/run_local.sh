#!/usr/bin/env bash
set -euo pipefail

http_addr="${POISED_HTTP_ADDR:-127.0.0.1:8080}"
database_url="${POISED_DATABASE_URL:-postgres://poised:poised@127.0.0.1:5432/poised?sslmode=disable}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

require_command go
require_command psql
require_command pg_isready

if ! pg_isready -h 127.0.0.1 -p 5432 >/dev/null 2>&1; then
  if command -v brew >/dev/null 2>&1 && brew list postgresql@16 >/dev/null 2>&1; then
    echo "Starting postgresql@16 with Homebrew..."
    brew services start postgresql@16 >/dev/null
  else
    echo "PostgreSQL is not reachable at 127.0.0.1:5432." >&2
    echo "Install/start PostgreSQL first, or set POISED_DATABASE_URL to an existing database." >&2
    exit 1
  fi
fi

if ! psql -d postgres -tAc "SELECT 1 FROM pg_roles WHERE rolname='poised'" | grep -q 1; then
  echo "Creating PostgreSQL role: poised"
  createuser poised
fi

psql -d postgres -v ON_ERROR_STOP=1 -c "ALTER ROLE poised WITH LOGIN PASSWORD 'poised';" >/dev/null

if ! psql -d postgres -tAc "SELECT 1 FROM pg_database WHERE datname='poised'" | grep -q 1; then
  echo "Creating PostgreSQL database: poised"
  createdb -O poised poised
fi

echo "Starting Poised..."
echo "UI:  http://${http_addr}/"
echo "API: http://${http_addr}/healthz"

export POISED_HTTP_ADDR="${http_addr}"
export POISED_DATABASE_URL="${database_url}"
export POISED_DATABASE_AUTO_MIGRATE="${POISED_DATABASE_AUTO_MIGRATE:-true}"
export POISED_DATABASE_MAX_CONNS="${POISED_DATABASE_MAX_CONNS:-5}"
export GOCACHE="${GOCACHE:-/tmp/poised-go-cache}"

go run ./cmd/poised -config configs/poised.example.json
