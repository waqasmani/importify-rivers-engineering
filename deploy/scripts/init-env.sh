#!/usr/bin/env bash
# Creates deploy/.env from .env.example with freshly generated secrets.
#
#   deploy/scripts/init-env.sh                       # local trial on http://localhost
#   deploy/scripts/init-env.sh rea.example.com       # public domain with automatic HTTPS
#
# Refuses to overwrite an existing deploy/.env.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="$ROOT/deploy/.env"
DOMAIN="${1:-}"

if [[ -e "$OUT" ]]; then
  echo "deploy/.env already exists; edit it instead (or move it away to regenerate)." >&2
  exit 1
fi
rand_b64() { head -c 32 /dev/urandom | base64 | tr -d '\n'; }
rand_hex() { head -c "$1" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

site="http://localhost"
public="http://localhost"
if [[ -n "$DOMAIN" ]]; then
  if [[ ! "$DOMAIN" =~ ^[A-Za-z0-9.-]+$ ]]; then
    echo "Pass a bare domain name such as rea.example.com" >&2
    exit 1
  fi
  site="$DOMAIN"
  public="https://$DOMAIN"
fi

umask 077
sed -e "s|^POSTGRES_PASSWORD=.*|POSTGRES_PASSWORD=$(rand_hex 24)|" \
    -e "s|^REA_SECRET_KEY=.*|REA_SECRET_KEY=$(rand_b64)|" \
    -e "s|^REA_SETUP_TOKEN=.*|REA_SETUP_TOKEN=$(rand_hex 16)|" \
    -e "s|^REA_SITE_ADDRESS=.*|REA_SITE_ADDRESS=$site|" \
    -e "s|^REA_PUBLIC_URL=.*|REA_PUBLIC_URL=$public|" \
    "$ROOT/.env.example" > "$OUT"
chmod 600 "$OUT"

echo "Wrote deploy/.env (mode 600)."
echo "Setup token for the first-run page:"
grep '^REA_SETUP_TOKEN=' "$OUT" | cut -d= -f2
echo "Back up REA_SECRET_KEY together with your database backups."
