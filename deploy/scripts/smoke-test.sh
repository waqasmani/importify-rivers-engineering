#!/usr/bin/env bash
# End-to-end check of a running Docker Compose deployment, used by CI and after upgrades:
#   web server and API health, security headers, first-run setup (if still pending), sign-in,
#   a CRM write, then backup → verify → restore, proving the restored data matches the backup.
#
#   deploy/scripts/smoke-test.sh            # uses REA_PUBLIC_URL and REA_SETUP_TOKEN from deploy/.env
#
# Restoring replaces the deployment's data, so only run this on a fresh or disposable stack.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need curl
need python3

base="${REA_SMOKE_URL:-$(envval REA_PUBLIC_URL)}"
base="${base%/}"
port="$(envval REA_HTTP_PORT)"
if [[ "$base" == "http://localhost" && -n "$port" && "$port" != "80" ]]; then base="http://localhost:$port"; fi
jar="$(mktemp)"
trap 'rm -f "$jar"' EXIT
json() { python3 -c "import json,sys; d=json.load(sys.stdin); print(eval(sys.argv[1], {}, {'d': d}))" "$1"; }

step() { echo "==> $*"; }

step "Waiting for $base/healthz"
for _ in $(seq 1 60); do
  curl -fsS "$base/healthz" >/dev/null 2>&1 && break
  sleep 2
done
curl -fsS "$base/healthz" | grep -q '"ok"' || die "API health check failed"
curl -fsS "$base/readyz" >/dev/null || die "API readiness check failed"

step "Dashboard is served with security headers"
headers="$(curl -fsSI "$base/")"
grep -qi '^content-security-policy:' <<<"$headers" || die "missing Content-Security-Policy on /"
grep -qi '^x-frame-options: *DENY' <<<"$headers" || die "missing X-Frame-Options on /"
curl -fsS "$base/companies" | grep -q '<div id="app"' || die "SPA fallback is not served"

email="smoke-owner@example.invalid"
password="smoke-$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
if [[ "$(curl -fsS "$base/api/v1/setup/status" | json 'd["needs_setup"]')" == "True" ]]; then
  step "Running first-run setup"
  token="$(envval REA_SETUP_TOKEN)"
  [[ -n "$token" ]] || die "REA_SETUP_TOKEN is not set in deploy/.env"
  body="$(python3 -c 'import json,sys; print(json.dumps({"setup_token":sys.argv[1],"organization_name":"Smoke Test","workspace_name":"Main","display_name":"Smoke Owner","email":sys.argv[2],"password":sys.argv[3]}))' "$token" "$email" "$password")"
  curl -fsS -c "$jar" -H 'Content-Type: application/json' -H "Origin: $base" -d "$body" "$base/api/v1/setup" >/dev/null || die "setup failed"
else
  [[ -n "${REA_SMOKE_EMAIL:-}" && -n "${REA_SMOKE_PASSWORD:-}" ]] || die "setup already done; set REA_SMOKE_EMAIL and REA_SMOKE_PASSWORD to sign in"
  email="$REA_SMOKE_EMAIL"
  password="$REA_SMOKE_PASSWORD"
fi

step "Signing in"
login="$(python3 -c 'import json,sys; print(json.dumps({"email":sys.argv[1],"password":sys.argv[2]}))' "$email" "$password")"
csrf="$(curl -fsS -c "$jar" -b "$jar" -H 'Content-Type: application/json' -H "Origin: $base" -d "$login" "$base/api/v1/auth/login" | json 'd["csrf_token"]')"
api() {
  local method="$1" path="$2" data="${3:-}"
  if [[ -n "$data" ]]; then
    curl -fsS -b "$jar" -X "$method" -H 'Content-Type: application/json' -H "X-CSRF-Token: $csrf" -H "Origin: $base" -d "$data" "$base/api/v1$path"
  else
    curl -fsS -b "$jar" -X "$method" -H "X-CSRF-Token: $csrf" -H "Origin: $base" "$base/api/v1$path"
  fi
}
company() { python3 -c 'import json,sys; print(json.dumps({"name":sys.argv[1],"company_type":"customer","status":"active","domain":None,"website":None,"industry":"","description":"","email":"","phone":"","address":"","city":"","country":"","employee_count":None,"owner_id":None,"custom_fields":{}}))' "$1"; }

step "Writing CRM data"
api POST /companies "$(company 'Before Backup Ltd')" >/dev/null || die "creating a company failed"

step "Backing up"
"$ROOT/deploy/scripts/backup.sh" | tee /dev/stderr | grep -q 'Backup written' || die "backup failed"
latest="$(find "$BACKUP_DIR" -maxdepth 1 -type d -name 'rea-*' | sort | tail -1)"

step "Verifying the backup in a temporary database"
"$ROOT/deploy/scripts/verify-backup.sh" "$latest"

step "Changing data after the backup"
api POST /companies "$(company 'After Backup Ltd')" >/dev/null

step "Restoring the backup"
"$ROOT/deploy/scripts/restore.sh" "$latest" --yes
for _ in $(seq 1 60); do
  curl -fsS "$base/readyz" >/dev/null 2>&1 && break
  sleep 2
done

step "Checking restored data"
csrf="$(curl -fsS -c "$jar" -b "$jar" -H 'Content-Type: application/json' -H "Origin: $base" -d "$login" "$base/api/v1/auth/login" | json 'd["csrf_token"]')"
names="$(api GET '/companies?limit=200' | json '",".join(sorted(c["name"] for c in d["items"]))')"
[[ "$names" == *"Before Backup Ltd"* ]] || die "restored database lacks data from before the backup"
[[ "$names" != *"After Backup Ltd"* ]] || die "restored database still contains data written after the backup"

echo "Smoke test passed: $base"
