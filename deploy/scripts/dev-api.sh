#!/usr/bin/env bash
# Runs the REA API for local development against a local PostgreSQL server.
#
#   deploy/scripts/dev-api.sh [database-name] [port]
#
# Creates the database if needed, applies migrations, and serves the built dashboard from
# apps/web/dist when it exists. Uses a fixed development setup token ("dev-setup-token") and a
# per-database secret key stored under .dev/<database>/. Never use this script in production.
set -euo pipefail

DB_NAME="${1:-rea_dev}"
PORT="${2:-8080}"
PGHOST="${PGHOST:-127.0.0.1}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-rea}"
PGPASSWORD="${PGPASSWORD:-rea}"
export PGHOST PGPORT PGUSER PGPASSWORD

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
STATE="$ROOT/.dev/$DB_NAME"
mkdir -p "$STATE/data"
if [[ ! -f "$STATE/secret-key" ]]; then
  head -c 32 /dev/urandom | base64 > "$STATE/secret-key"
  chmod 600 "$STATE/secret-key"
fi

if ! psql -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = '$DB_NAME'" | grep -q 1; then
  createdb "$DB_NAME"
fi

export REA_DATABASE_URL="postgres://$PGUSER:$PGPASSWORD@$PGHOST:$PGPORT/$DB_NAME?sslmode=disable"
export REA_HTTP_ADDR="127.0.0.1:$PORT"
export REA_DATA_DIR="$STATE/data"
export REA_SECRET_KEY="$(cat "$STATE/secret-key")"
export REA_SETUP_TOKEN="${REA_SETUP_TOKEN:-dev-setup-token}"
export REA_COOKIE_SECURE=false
export REA_LOG_FORMAT=text
export REA_PUBLIC_URL="${REA_PUBLIC_URL:-http://127.0.0.1:$PORT}"
if [[ -z "${REA_WEB_DIR:-}" && -d "$ROOT/apps/web/dist" ]]; then
  export REA_WEB_DIR="$ROOT/apps/web/dist"
fi

if [[ -n "${REA_API_BIN:-}" ]]; then
  exec "$REA_API_BIN" serve
fi
cd "$ROOT"
exec go run ./services/api serve
