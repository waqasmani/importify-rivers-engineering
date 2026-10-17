#!/usr/bin/env bash
# Starts a throw-away API for end-to-end tests: recreates the database, clears its data directory
# and serves the built dashboard from apps/web/dist. Used by the Playwright configs of apps/web and
# apps/extension.
#
#   deploy/scripts/e2e-api.sh [port] [database]
set -euo pipefail
PORT="${1:-18190}"
DB_NAME="${2:-rea_e2e}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
export PGHOST="${PGHOST:-127.0.0.1}" PGPORT="${PGPORT:-5432}" PGUSER="${PGUSER:-rea}" PGPASSWORD="${PGPASSWORD:-rea}"

dropdb --if-exists --force "$DB_NAME"
createdb "$DB_NAME"
rm -rf "$ROOT/.dev/$DB_NAME/data"
# Serve the built dashboard when present (the dashboard tests need it; the extension tests do not).
if [[ -f "$ROOT/apps/web/dist/index.html" ]]; then export REA_WEB_DIR="$ROOT/apps/web/dist"; fi
exec "$ROOT/deploy/scripts/dev-api.sh" "$DB_NAME" "$PORT"
