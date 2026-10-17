#!/usr/bin/env bash
# Starts a throw-away API for end-to-end tests: recreates the database, clears its data directory
# and serves the built dashboard from apps/web/dist. Used by apps/web/playwright.config.ts.
#
#   deploy/scripts/e2e-api.sh [port] [database]
set -euo pipefail
PORT="${1:-18190}"
DB_NAME="${2:-rea_e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PGHOST="${PGHOST:-127.0.0.1}" PGPORT="${PGPORT:-5432}" PGUSER="${PGUSER:-rea}" PGPASSWORD="${PGPASSWORD:-rea}"

[[ -f "$ROOT/apps/web/dist/index.html" ]] || { echo "Build the dashboard first: pnpm --filter @rea/web build" >&2; exit 1; }
dropdb --if-exists --force "$DB_NAME"
createdb "$DB_NAME"
rm -rf "$ROOT/.dev/$DB_NAME/data"
export REA_WEB_DIR="$ROOT/apps/web/dist"
exec "$ROOT/deploy/scripts/dev-api.sh" "$DB_NAME" "$PORT"
