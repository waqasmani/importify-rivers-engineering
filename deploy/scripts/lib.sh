# Shared helpers for the operator scripts. Source it; do not run it.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
COMPOSE_FILE="${REA_COMPOSE_FILE:-$ROOT/deploy/docker-compose.yml}"
BACKUP_DIR="${REA_BACKUP_DIR:-$ROOT/deploy/backups}"

compose() { docker compose -f "$COMPOSE_FILE" "$@"; }

die() {
  echo "error: $*" >&2
  exit 1
}

need() { command -v "$1" >/dev/null 2>&1 || die "$1 is required"; }

# Reads a variable from deploy/.env without sourcing it.
envval() {
  local file="$ROOT/deploy/.env"
  [[ -f "$file" ]] || return 0
  grep -E "^$1=" "$file" | tail -1 | cut -d= -f2-
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}
