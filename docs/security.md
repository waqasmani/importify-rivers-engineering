# Security and responsible analysis

This document maps REA's security and responsible-analysis requirements to the code that enforces
them and the tests that check it. Report vulnerabilities privately to the repository owner.

## Responsible analysis

| Requirement | How REA enforces it | Where |
| --- | --- | --- |
| Content is collected only through permitted access and user-initiated workflows | The extension reads a page only after the user clicks capture (activeTab) or allows a site; the server fetches URLs only when `REA_SERVER_FETCH_ENABLED=true`, one page per source (no crawling), honouring robots.txt | `apps/extension`, `services/internal/safefetch` |
| Source code is analyzed only when the user owns it or is authorized | Every analysis source requires an explicit authorization confirmation and basis, stored with the source and shown in the UI | `httpapi/analysis.go`, `analysis_sources` table |
| Credentials, cookies, access tokens and passwords are not extracted from target pages | The extension never reads `document.cookie`, skips all form field values, drops password and hidden inputs and token-like meta tags; snapshots are sanitized again on the server; recorded API traffic has `Authorization`, `Cookie`, `Set-Cookie` and token-like fields removed before storage; codebase analysis never reads `.env`, key files or environment variable values | `apps/extension/src/content`, `services/internal/htmlx`, `services/internal/analysis/openapi.go`, `codebase.go` |
| Private endpoints are not accessed without authorization | Server fetches reject private, loopback, link-local and metadata addresses at dial time (after DNS resolution, defeating DNS rebinding) unless an administrator opts in | `safefetch` |
| Authentication controls, paywalls, rate limits and access restrictions are not bypassed | Server fetches send no cookies or credentials, follow robots.txt, and stop on any HTTP error status (401, 403, 429, …) with "REA does not bypass access restrictions", without retrying; the extension only sees what the signed-in user's browser shows | `safefetch`, extension |
| Arbitrary commands are not run and uploaded code is not executed | Analyzers only read files as data (source text, package manifests, OpenAPI YAML/JSON, PDF and DOCX text); there is no build, install or run step anywhere | `services/internal/analysis` |
| Uploads are size-limited and validated | `REA_MAX_UPLOAD_MB`, content sniffing, executable extensions rejected, filenames sanitized, files stored under random keys | `httpapi/activity.go`, `filestore` |
| ZIP files are protected against path traversal and decompression bombs | Absolute paths, `..`, links and device files rejected; limits on entry count, per-file and total uncompressed size and compression ratio | `safearchive` (+ tests) |
| Visible-only sources are not over-interpreted | Website and screenshot analyzers only emit findings about visible behaviour; anything about hidden implementation is classified `inferred` or `unknown` with stated assumptions | `analysis/website.go`, `document.go` |
| AI-generated reports are identified | Reports and findings carry `origin` (`rule`, `ai`, `manual`), AI reports have `is_ai_generated`, provider and model; the UI labels them and every AI result requires an explicit apply step | `httpapi/ai.go`, `components/ai` |

## Application security

| Control | Implementation |
| --- | --- |
| Passwords | Argon2id (64 MiB, t=2, p=2, 16-byte salt), minimum 12 characters, constant-time verification, dummy hash on unknown users to avoid user enumeration by timing. |
| Sessions | Random 256-bit tokens stored as SHA-256 hashes; `HttpOnly`, `SameSite=Lax`, `Secure` and `__Host-` prefix on HTTPS; absolute and idle expiry; listed and revocable in Profile; all sessions revoked on password reset. |
| CSRF | Per-session token required in `X-CSRF-Token` for every unsafe cookie-authenticated request; JSON-only bodies (`415` otherwise) so simple cross-site forms cannot post. |
| Brute force | Per-IP rate limits (API 1200/min, sign-in and setup 20/min) and account lockout after 10 failures for 15 minutes. |
| Authorization | Role-based permissions checked on every route and again per record type for notes, tags, files, tasks and AI jobs; the UI hides what a role cannot do. |
| Least-privilege roles | Built-in admin, analyst, member and viewer roles; custom roles from the permission catalogue. |
| Personal API tokens | Prefixed (`reap_`), shown once, stored as SHA-256 hashes, scoped to the creator's permissions, revocable. |
| Integration keys | For machine clients reading the catalog. Created only by administrators (`settings.manage`) in a browser session; prefixed (`reai_`), shown once, stored as SHA-256 hashes, never logged (request logs carry the key ID); optional expiry; revocable with immediate effect. Organization-owned with fixed scopes (`catalog.read`, `catalog.read_costs`, `imports.write`) that are never combined with a user's role, so changing or deleting the creator neither widens nor breaks a key. Refused on every endpoint except the catalog and product import submission, before routing and again by permission checks; imports from keys always go to review and cannot overwrite existing products. Rate limited per key (`REA_INTEGRATION_RATE_LIMIT`); requests counted per day; first use per hour and every new IP audited; writes attributed to the key. Cost, supplier and source fields need `catalog.read_costs`. |
| Webhooks | HTTPS only (plain http only to localhost with `REA_WEBHOOK_ALLOW_HTTP_LOCALHOST`). Outbound requests check the resolved address at dial time (DNS rebinding safe) and refuse private, loopback, link-local, CGNAT and reserved ranges unless `REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS` is set; cloud metadata addresses are always refused; no proxies, no redirects, timeouts and response size caps. Signing secrets are generated by REA, shown once, encrypted with AES-256-GCM (`REA_SECRET_KEY`) and rotatable; deliveries are signed with HMAC-SHA256 over `timestamp.body`. Repeatedly failing endpoints are disabled with a visible, audited reason. |
| Marketplace definitions and rules adapters | Rule documents are data validated by the same schema in the extractor (zod) and the API (Go): no code, no regular expressions, no requests, no storage; paths cannot reach prototypes or token-like keys, attributes cannot be `value`, `on…` or token-like, form controls are never read; embedded script data is parsed (never evaluated) with size (2 MB per script) and depth (64) limits and redacted before use. The Adapter lab parses saved pages with `DOMParser`, never inserts them into the dashboard and never loads their resources (`packages/extractor/src/rules`, `services/internal/marketplaces`). |
| Browser extension | Pairing codes valid 10 minutes and once; access tokens 1 hour; refresh tokens 30 days, rotated on every use, reuse revokes the device; scopes are intersected with the user's current role; tokens never reach page scripts; devices are listed and revocable. |
| CORS | Cookie requests only from `REA_PUBLIC_URL` and configured origins; extension origins may use bearer tokens only, never cookies; everything else gets no CORS headers. |
| Security headers | CSP (`script-src 'self'`, `frame-ancestors 'none'`, `object-src 'none'`), `X-Frame-Options: DENY`, `nosniff`, strict referrer policy, COOP, Permissions-Policy, HSTS on HTTPS; downloads use `Content-Disposition: attachment` with a sandbox CSP. |
| Input validation | Strict JSON decoding (unknown fields rejected), field-level validation with limits, UUID path parameters, enumerated values, sanitized HTML, CSV formula-injection protection on export. |
| Secrets | Only in environment variables; AI keys encrypted with AES-256-GCM using `REA_SECRET_KEY`, write-only in the API; the audit log and structured logs redact passwords, tokens, cookies and keys; `.gitignore` excludes `.env` files and keys. |
| External AI | Off until an administrator adds a provider. Non-local providers need an administrator's data-sharing permission and a per-request user confirmation naming the host and data categories. Only evidence REA already stores for the record is sent; prompts mark evidence as untrusted data to resist prompt injection. |
| Audit | Sign-in, setup, user and role changes, settings, record writes and deletes, merges, imports, AI runs and applies, exports, backups, extension pairing/revocation, integration keys (create, revoke, use) and webhooks (create, update, rotate, test, redeliver, automatic disable) are recorded with actor (user, device or integration key), IP, request ID and field changes. |
| Data ownership | Users export their own data (Profile) and administrators export everything (System); records, captures and analysis projects can be deleted; capture retention purges old snapshots. |
| Containers | API runs as an unprivileged user on a read-only root filesystem with all capabilities dropped and `no-new-privileges`; only the web server publishes ports. |

## Security regression tests

`go test ./services/...` includes, among others: CSRF and content-type enforcement, CORS rules for
web and extension origins, security headers, permission checks per role, login lockout and rate
limiting, extension pairing, refresh rotation, reuse detection and revocation, audit redaction,
archive traversal and bomb rejection, SSRF blocking including literal private IPs, credential
stripping from recorded API traffic, HTML sanitization, attachment download headers, AI external
data gating and grounding, and that the committed OpenAPI document matches the server. The catalog
tests check that integration keys cannot reach admin, CRM, settings or profile endpoints, that
costs are hidden without `catalog.read_costs`, revocation and expiry, per-key rate limits, change
capture on every write path, webhook signatures verified with an independent HMAC, retries,
backoff and automatic disabling against a local server, SSRF blocking at save and dial time, and
that webhook secrets are encrypted at rest.
Extension tests check that the login-page fixture yields no password, token or cookie values.
