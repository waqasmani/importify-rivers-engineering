#!/bin/sh
# Writes optional global options for the Caddyfile, then starts Caddy. The Caddyfile imports
# /config/rea-global*.caddy, which matches nothing when no option is set.
set -eu
rm -f /config/rea-global.caddy
if [ -n "${REA_ACME_EMAIL:-}" ]; then
  case "$REA_ACME_EMAIL" in
    *[!A-Za-z0-9.@+_-]*) echo "REA_ACME_EMAIL contains unexpected characters" >&2; exit 1 ;;
  esac
  printf 'email %s\n' "$REA_ACME_EMAIL" > /config/rea-global.caddy
fi
exec caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
