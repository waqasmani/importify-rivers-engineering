# Configuration

REA reads all configuration from environment variables. Secrets never live in source control or
images. With Docker Compose, put values in `deploy/.env` (created by `deploy/scripts/init-env.sh`
from the commented template `.env.example`). Without Docker, export them before starting `rea-api`.

## API and worker (`rea-api`, `rea-worker`)

| Variable | Default | Purpose |
| --- | --- | --- |
| `REA_DATABASE_URL` | required | PostgreSQL connection URL, e.g. `postgres://rea:PASSWORD@db:5432/rea?sslmode=disable`. |
| `REA_SECRET_KEY` | required | 32 random bytes, base64 (`openssl rand -base64 32`). Encrypts stored AI provider keys and webhook signing secrets (AES-256-GCM). Keep it with your backups. |
| `REA_PUBLIC_URL` | `http://localhost:8080` | URL users and the extension use. Its origin is allowed for cookie requests; `https` turns on secure cookies and HSTS. |
| `REA_HTTP_ADDR` | `:8080` | Listen address. The container image sets `0.0.0.0:8080`. |
| `REA_DATA_DIR` | `./data` | Attachments, snapshots, uploaded sources, API-created backups and the setup token file. |
| `REA_WEB_DIR` | empty | Serve a built dashboard from this directory (single-binary installs). Compose uses Caddy instead. |
| `REA_SETUP_TOKEN` | generated | One-time token for first-run setup. If empty, the API writes a random token to `$REA_DATA_DIR/setup-token` (never to logs) and deletes it after setup. |
| `REA_ALLOWED_ORIGINS` | empty | Extra comma-separated origins. `https://…` origins may use cookies; `chrome-extension://<id>` origins may use extension bearer tokens. The default extension ID `olceibeplclcegianhjffambadoakkno` is always allowed. |
| `REA_COOKIE_SECURE` | from `REA_PUBLIC_URL` | Force secure cookies on (`true`) or off (`false`). |
| `REA_TRUSTED_PROXIES` | empty | Comma-separated IPs or CIDRs whose `X-Forwarded-For` is trusted for client IPs (rate limiting, lockout, audit). Compose sets the Docker subnet. |
| `REA_SESSION_TTL` | `168h` | Absolute session lifetime. |
| `REA_SESSION_IDLE` | `24h` | Idle timeout for sessions. |
| `REA_ACCESS_TOKEN_TTL` | `1h` | Extension access token lifetime. |
| `REA_REFRESH_TOKEN_TTL` | `720h` | Extension refresh token lifetime (rotated on every use). |
| `REA_MAX_UPLOAD_MB` | `25` | Maximum size of one uploaded file. |
| `REA_MAX_ARCHIVE_UNCOMPRESSED_MB` | `300` | Maximum total uncompressed size of a repository archive. |
| `REA_MAX_ARCHIVE_FILES` | `20000` | Maximum number of entries in a repository archive. |
| `REA_SERVER_FETCH_ENABLED` | `false` | Allow website analysis to fetch public URLs from the server (robots.txt honoured, rate limited). |
| `REA_FETCH_ALLOW_PRIVATE_NETWORKS` | `false` | Allow server fetches to private, loopback and link-local addresses. Leave off unless you analyze your own intranet sites. |
| `REA_REPO_ROOTS` | empty | Comma-separated directories that may be analyzed as repository paths. Empty disables repository-path sources. |
| `REA_INTEGRATION_RATE_LIMIT` | `600` | Requests per minute allowed for each integration key (catalog API), on top of the per-IP limit. |
| `REA_CATALOG_CHANGES_RETENTION_DAYS` | `90` | How long catalog change-feed entries are kept. Clients with an older cursor get `410` and reload the catalog. |
| `REA_WEBHOOK_ALLOW_PRIVATE_NETWORKS` | `false` | Allow webhook targets on private, loopback, link-local and other reserved addresses (cloud metadata addresses stay blocked). Only for receivers on your own network. |
| `REA_WEBHOOK_ALLOW_HTTP_LOCALHOST` | `false` | Allow plain `http://` webhook URLs to `localhost` (and loopback connections for them). Development only. |
| `REA_WEBHOOK_TIMEOUT` | `10s` | Time limit for one delivery attempt (at most `2m`). |
| `REA_WEBHOOK_MAX_ATTEMPTS` | `10` | Attempts per delivery before it is marked failed (at most 50). |
| `REA_WEBHOOK_RETRY_BASE` | `1m` | Delay before the first retry; it doubles per attempt (±20 % jitter). |
| `REA_WEBHOOK_RETRY_MAX` | `6h` | Longest delay between two attempts. |
| `REA_WEBHOOK_DISABLE_AFTER_FAILURES` | `20` | Consecutive failed attempts after which an endpoint is disabled automatically... |
| `REA_WEBHOOK_DISABLE_FAILING_FOR` | `1h` | ...provided it has been failing for at least this long. The reason is shown on the endpoint. |
| `REA_WEBHOOK_CONCURRENCY` | `2` | Parallel webhook senders per worker (separate from analysis and AI jobs). |
| `REA_WEBHOOK_LOG_RETENTION_DAYS` | `30` | How long finished webhook deliveries (the delivery log) are kept. |
| `REA_WORKER_MODE` | `embedded` | `embedded` runs analysis and AI jobs inside the API. `off` leaves them to `rea-worker`. |
| `REA_WORKER_CONCURRENCY` | `2` | Parallel jobs per worker. |
| `REA_AUTO_MIGRATE` | `true` | Apply pending migrations at API start-up. Set `false` to run `rea-api migrate up` yourself. |
| `REA_PG_DUMP` | `pg_dump` | pg_dump used by the in-app backup button. |
| `REA_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `REA_LOG_FORMAT` | `json` | `json` or `text`. Logs never contain passwords, tokens, cookies or API keys. |
| `REA_VERSION` | build version | Version string shown in System settings. |

## Docker Compose only

| Variable | Default | Purpose |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | required | Database password, used by the `db` service and in `REA_DATABASE_URL`. |
| `REA_SITE_ADDRESS` | `http://localhost` | Caddy site address: a domain for automatic HTTPS, `https://localhost`, `http://localhost`, or `:80` behind another proxy. |
| `REA_ACME_EMAIL` | empty | Contact e-mail for Let's Encrypt. |
| `REA_HTTP_PORT`, `REA_HTTPS_PORT` | `80`, `443` | Published ports of the web server. |
| `REA_BIND_ADDRESS` | `0.0.0.0` | Host address the ports bind to. Use `127.0.0.1` behind a host-level proxy. |
| `REA_PROXY_MAX_BODY` | `600MB` | Request body limit at the web server. |
| `REA_DOCKER_SUBNET` | `172.29.10.0/24` | Subnet of the stack's network; trusted for forwarded client IPs. Change it if it collides. |
| `REA_UPSTREAM_TRUSTED_PROXIES` | `private_ranges` | Proxies in front of Caddy whose forwarded headers Caddy trusts. |
| `REA_BACKUP_KEEP` | `14` | Backups kept by `deploy/scripts/backup.sh`. |

## Settings stored in the database

Organization name, CRM labels (for example renaming "Suppliers" to "Vendors"), import defaults
(currency, title similarity threshold, supplier auto-linking), capture options (HTML snapshots,
snapshot size, retention) and the AI on/off switch are edited by administrators under
**Settings** in the dashboard and stored in `app_settings`. AI providers are configured under
**AI providers**; their API keys are encrypted with `REA_SECRET_KEY` and never returned by the API.
Integration keys and webhook endpoints are managed under **Settings → Integrations**; webhook
signing secrets are encrypted with `REA_SECRET_KEY` as well (see [catalog-api.md](catalog-api.md)).
