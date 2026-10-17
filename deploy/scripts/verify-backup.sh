#!/usr/bin/env bash
# Proves a backup can be restored without touching the live database:
#   1. checks the SHA-256 checksums,
#   2. restores database.dump into a temporary database on the running db service,
#   3. checks the schema version, core tables and row counts,
#   4. lists the data archive, then drops the temporary database.
#
#   deploy/scripts/verify-backup.sh deploy/backups/rea-20260101-020000
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need docker
dir="${1:-}"
[[ -n "$dir" && -d "$dir" ]] || die "usage: $0 <backup directory>"
dir="$(cd "$dir" && pwd)"

echo "Checking checksums…"
(cd "$dir" && sha256 -c SHA256SUMS) || die "checksum mismatch"

tmpdb="rea_verify_$(date +%s)"
cleanup() { compose exec -T db dropdb -U rea --if-exists "$tmpdb" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "Restoring into temporary database $tmpdb…"
compose exec -T db createdb -U rea "$tmpdb"
compose exec -T db pg_restore -U rea -d "$tmpdb" --no-owner --exit-on-error < "$dir/database.dump"

q() { compose exec -T db psql -U rea -d "$tmpdb" -Atc "$1" | tr -d '\r'; }
schema="$(q 'SELECT max(version_id) FROM goose_db_version')"
expected="$(grep '^schema_version=' "$dir/manifest.txt" | cut -d= -f2)"
[[ "$schema" == "$expected" ]] || die "schema version $schema does not match manifest $expected"
for t in users roles permissions companies contacts suppliers products product_imports analysis_projects analysis_findings audit_logs; do
  n="$(q "SELECT count(*) FROM $t")" || die "table $t missing in restored database"
  printf '  %-20s %s rows\n' "$t" "$n"
done
[[ "$(q 'SELECT count(*) FROM users')" -ge 1 ]] || die "restored database has no users"

echo "Checking data archive…"
files="$(tar -tzf "$dir/data.tar.gz" | wc -l | tr -d ' ')"
echo "  $files entries"

echo "Backup verified: schema version $schema."
