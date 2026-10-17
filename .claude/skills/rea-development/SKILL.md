---
name: rea-development
description: Build, extend, test and deploy the REA Importify platform (REA "Reverse Engineering Anything" analysis engine, Importify CRM, product import manager, Chrome MV3 extension; Go + PostgreSQL + Vue 3). Use when changing code in this repository — adding API endpoints, migrations, sqlc queries, dashboard screens, analyzers, AI tools, extension site adapters — or when running its tests, Docker Compose stack, backups or releases.
---

# REA Importify development

REA Importify is a self-hosted monorepo. Read `references/architecture.md` for the map and
`references/recipes.md` for step-by-step changes. The non-negotiable product and security rules
are in `references/rules.md`; check every change against them.

## Layout

| Path | What lives there |
| --- | --- |
| `services/api`, `services/worker` | `rea-api` (HTTP, migrations, operator commands) and `rea-worker` |
| `services/internal/httpapi` | chi routes, middleware, handlers, OpenAPI generation (`openapi.go` `Operations()`) |
| `services/internal/store` | sqlc output from `db/queries/*.sql` — never edit by hand |
| `services/internal/{analysis,ai,products,normalize,safefetch,safearchive,htmlx,auth,worker}` | domain packages |
| `db/migrations` | goose migrations, embedded, applied at start-up |
| `packages/api-types` | committed `openapi.json` + generated TypeScript (`Schemas['Name']`) |
| `apps/web` | Vue 3 dashboard (see `docs/web-conventions.md`) |
| `apps/extension` | Chrome MV3 extension |
| `deploy` | Compose stack, Caddy/nginx, `init-env.sh`, `backup.sh`, `verify-backup.sh`, `restore.sh`, `smoke-test.sh`, `dev-api.sh` |

## Everyday commands

```bash
make deps                                   # go mod download + pnpm install
export REA_TEST_DATABASE_URL='postgres://rea:rea@127.0.0.1:5432/rea?sslmode=disable'
make test-go                                # Go unit + DB integration + contract + migration tests
make test-web test-extension                # Vitest suites
make lint                                   # gofmt, vet, migration lint, TypeScript
make generate                               # sqlc + openapi.json + TS types after API changes
make dev-api   # API :8080, setup token "dev-setup-token"
make dev-web   # dashboard :5173 proxying /api
make e2e       # Playwright against a real API
docker compose -f deploy/docker-compose.yml up -d --build --wait
```

## Definition of done for any change

1. Code formatted (`gofmt -w services db`), typed (`pnpm -r typecheck`), no TODOs, mocks,
   placeholder buttons or demo data; every UI action calls the real API.
2. Tests added next to the change and passing: Go handler tests use `testutil` (real PostgreSQL,
   one cloned database per test) and the contract validator; dashboard tests use
   `apps/web/tests/unit/helpers.ts`; extension tests use the shared `testdata/fixtures`.
3. New routes are registered in `Operations()`; `make generate` leaves no diff.
4. Migrations are reversible and non-destructive in Up (the migration tests enforce it).
5. Security rules in `references/rules.md` still hold; audit events written for state changes.
6. Report results honestly: say what ran, what passed, and what could not be verified.
