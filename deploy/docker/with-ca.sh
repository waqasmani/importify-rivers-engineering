#!/bin/sh
# Runs a build command that also trusts an optional extra CA certificate, for builds behind a
# TLS-intercepting corporate proxy. Pass it as a BuildKit secret:
#   docker build --secret id=extra_ca,src=/path/to/proxy-ca.crt ...
# Without the secret the command runs unchanged. Nothing from this script stays in the image.
set -e
bundle=""
if [ -s /run/secrets/extra_ca ]; then
  bundle="$(mktemp)"
  if [ -f /etc/ssl/certs/ca-certificates.crt ]; then cat /etc/ssl/certs/ca-certificates.crt >> "$bundle"; fi
  cat /run/secrets/extra_ca >> "$bundle"
  export SSL_CERT_FILE="$bundle" NODE_EXTRA_CA_CERTS=/run/secrets/extra_ca GIT_SSL_CAINFO="$bundle"
fi
status=0
"$@" || status=$?
if [ -n "$bundle" ]; then rm -f "$bundle"; fi
exit "$status"
