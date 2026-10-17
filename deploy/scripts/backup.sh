#!/usr/bin/env bash
# Creates a consistent backup of a Docker Compose deployment:
#   deploy/backups/rea-YYYYmmdd-HHMMSS/
#     database.dump    pg_dump custom format (all tables, including users and settings)
#     data.tar.gz      the API data directory (attachments, snapshots, uploaded sources)
#     manifest.txt     versions, sizes and SHA-256 checksums
#
#   deploy/scripts/backup.sh            # keeps the newest REA_BACKUP_KEEP backups (default 14)
#
# The database is dumped while the stack runs (pg_dump takes a consistent snapshot). Secrets in
# deploy/.env are NOT included; store REA_SECRET_KEY separately and securely.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need docker

stamp="$(date -u +%Y%m%d-%H%M%S)"
dest="$BACKUP_DIR/rea-$stamp"
mkdir -p "$dest"
chmod 700 "$BACKUP_DIR" "$dest"

compose ps --status running db >/dev/null 2>&1 || die "the db service is not running"

echo "Dumping database…"
compose exec -T db pg_dump -U rea -d rea --format=custom --compress=6 --no-owner > "$dest/database.dump"
[[ -s "$dest/database.dump" ]] || die "database dump is empty"

echo "Archiving data directory…"
compose exec -T api tar -C /data --exclude=./backups --exclude=./setup-token -czf - . > "$dest/data.tar.gz"
tar -tzf "$dest/data.tar.gz" >/dev/null || die "data archive is unreadable"

schema="$(compose exec -T db psql -U rea -d rea -Atc 'SELECT max(version_id) FROM goose_db_version' | tr -d '\r')"
{
  echo "created_utc=$stamp"
  echo "rea_version=$(envval REA_VERSION)"
  echo "schema_version=$schema"
  echo "postgres=$(compose exec -T db postgres --version | tr -d '\r')"
  echo "database_bytes=$(wc -c < "$dest/database.dump" | tr -d ' ')"
  echo "data_bytes=$(wc -c < "$dest/data.tar.gz" | tr -d ' ')"
} > "$dest/manifest.txt"
(cd "$dest" && sha256 database.dump data.tar.gz > SHA256SUMS)
chmod 600 "$dest"/*

keep="$(envval REA_BACKUP_KEEP)"
keep="${keep:-14}"
mapfile -t old < <(find "$BACKUP_DIR" -maxdepth 1 -type d -name 'rea-*' | sort -r | tail -n +$((keep + 1)))
for d in "${old[@]}"; do
  rm -rf -- "$d"
  echo "Removed old backup $(basename "$d")"
done

echo "Backup written to $dest"
echo "Verify it with: deploy/scripts/verify-backup.sh $dest"
