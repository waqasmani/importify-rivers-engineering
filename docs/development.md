# Development

## Toolchain

| Tool | Version |
| --- | --- |
| Go | 1.27.2 |
| Node.js | 22 LTS |
| pnpm | 10.28.0 (`corepack enable` or `npm i -g pnpm@10.28.0`) |
| PostgreSQL | 18 (16+ works for development; the extensions `pg_trgm` and `citext` are required) |
| sqlc | 1.31.1 (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`) |
| Docker | 24+ with Compose (for the full stack and image builds) |

## First run

```bash
git clone https://github.com/waqasmani/importify-rivers-engineering.git rea && cd rea
make deps

# A development database (any local PostgreSQL where role "rea" may create databases):
sudo -u postgres psql -c "CREATE ROLE rea LOGIN CREATEDB PASSWORD 'rea'" -c "CREATE DATABASE rea OWNER rea"

# Terminal 1: API on http://127.0.0.1:8080 (creates database rea_dev, setup token "dev-setup-token")
make dev-api
# Terminal 2: dashboard with hot reload on http://127.0.0.1:5173
make dev-web
```

Open http://127.0.0.1:5173, enter the setup token `dev-setup-token` and create the owner account.
`deploy/scripts/dev-api.sh <database> <port>` runs further isolated instances; it keeps a
per-database secret key under `.dev/` (git-ignored).

## Tests

```bash
export REA_TEST_DATABASE_URL='postgres://rea:rea@127.0.0.1:5432/rea?sslmode=disable'
make test-go          # unit, database integration, API contract, migration lint and rollback, security regressions
make test-web         # Vue component tests (Vitest + happy-dom)
make test-extractor   # @rea/extractor: adapters, rules engine, every marketplace definition against its fixture
make test-extension   # extension extraction entry, sanitizer, messaging and token logic
make e2e              # Playwright: dashboard flows and extension load/pairing against a real API
make lint             # gofmt, go vet, migration lint, TypeScript
```

Database tests clone a migrated template database per test, so they are isolated and parallel.
The backup test runs `REA_PG_DUMP` (default `pg_dump`), whose major version must be at least the
server's: with PostgreSQL 18 install `postgresql-client-18` and set
`REA_PG_DUMP=/usr/lib/postgresql/18/bin/pg_dump`, as CI does.
Without `REA_TEST_DATABASE_URL` they are skipped; CI sets `REA_REQUIRE_DB_TESTS=1` so a missing
database fails instead. Fixtures for product pages, OpenAPI, HAR files and documents live in
`testdata/fixtures` and are shared by the Go and extension tests.

## Changing the API

1. Edit SQL in `db/queries/*.sql` (and a new migration in `db/migrations` if the schema changes;
   migrations must be reversible and may not drop data in their Up section, see
   `services/internal/migrate/migrate_test.go`).
2. `make generate` regenerates sqlc code, the OpenAPI document and TypeScript types.
3. Register new routes in the `Operations()` table in `services/internal/httpapi/openapi.go` so
   they are documented; the coverage test fails for undocumented routes.
4. Use `Schemas['TypeName']` from `@/api/client` in the dashboard; see `docs/web-conventions.md`.

## Repository layout

```
apps/web               dashboard
apps/extension         browser extension
services/api           rea-api command
services/worker        rea-worker command
services/internal      Go packages
packages/api-types     OpenAPI document and TypeScript types
db/migrations          goose migrations (embedded)
db/queries             sqlc queries
deploy                 docker-compose.yml, proxy, scripts, systemd
docs                   documentation
testdata/fixtures      shared test fixtures
```
