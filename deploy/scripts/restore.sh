#!/usr/bin/env bash
# Restores a backup made by backup.sh into the Docker Compose deployment. DESTRUCTIVE: replaces
# the current database and data directory. Run verify-backup.sh first.
#
#   deploy/scripts/restore.sh deploy/backups/rea-20260101-020000 [--yes]
#   deploy/scripts/restore.sh rea-20260101-020000.dump [--yes]     # database-only backup from
#                                                                    # Settings → System
#
# Steps: verify checksums → stop api/worker/web → recreate the database → pg_restore → replace
# the data directory → start the stack (the API applies newer migrations automatically).
# deploy/.env must contain the same REA_SECRET_KEY as when the backup was made, otherwise stored
# AI provider keys cannot be decrypted (re-enter them in AI settings).
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need docker
src="${1:-}"
[[ -n "$src" && -e "$src" ]] || die "usage: $0 <backup directory | file.dump> [--yes]"
if [[ -d "$src" ]]; then
  dir="$(cd "$src" && pwd)"
  dump="$dir/database.dump"
  data="$dir/data.tar.gz"
  [[ -f "$dump" && -f "$data" ]] || die "$dir is not a REA backup"
  (cd "$dir" && sha256 -c SHA256SUMS) || die "checksum mismatch; refusing to restore"
  when="$(grep '^created_utc=' "$dir/manifest.txt" | cut -d= -f2)"
else
  dump="$(cd "$(dirname "$src")" && pwd)/$(basename "$src")"
  data=""
  when="$(basename "$src")"
  echo "Database-only backup: attachments and uploaded files are left as they are."
fi

if [[ "${2:-}" != "--yes" ]]; then
  echo "This replaces ALL current REA data with the backup from $when."
  read -r -p "Type RESTORE to continue: " answer
  [[ "$answer" == "RESTORE" ]] || die "cancelled"
fi

echo "Stopping application services…"
compose stop web worker api >/dev/null 2>&1 || true
compose up -d db
for _ in $(seq 1 30); do
  compose exec -T db pg_isready -U rea -d postgres >/dev/null 2>&1 && break
  sleep 2
done

echo "Recreating database…"
compose exec -T db psql -U rea -d postgres -v ON_ERROR_STOP=1 -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'rea' AND pid <> pg_backend_pid();" >/dev/null
compose exec -T db dropdb -U rea --if-exists rea
compose exec -T db createdb -U rea rea
compose exec -T db pg_restore -U rea -d rea --no-owner --exit-on-error < "$dump"

if [[ -n "$data" ]]; then
  echo "Restoring data directory…"
  compose run --rm --no-deps -T --entrypoint sh api -c 'find /data -mindepth 1 -maxdepth 1 ! -name backups -exec rm -rf {} + && tar -C /data -xzf -' < "$data"
fi

echo "Starting services…"
compose up -d
echo "Restore finished. Sign in and check recent records; the audit log continues from the backup."
