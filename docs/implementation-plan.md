# Implementation plan (Phase 1 assessment)

## Repository state at start

The repository contained only a one-line `README.md` and a single commit. There was no existing
code, build tooling, database, or environment configuration to preserve, so the platform is
built from scratch using the layout requested in the specification.

## Verified toolchain (checked 2026-10-16)

| Component | Version chosen | Notes |
|---|---|---|
| Go | 1.27.2 (`toolchain` directive) | Container ships 1.24.7, which is out of support. Current `x/crypto`, `x/net` and `goose` require Go 1.26+. |
| chi | v5.5.0 | HTTP router |
| pgx | v5.11.0 | PostgreSQL driver and pool |
| sqlc | v1.31.1 | Type-safe query generation |
| goose | v3.29.0 | Migrations, embedded in the API binary |
| PostgreSQL | 18 (Docker), 16+ supported | Uses only `pg_trgm` and `citext`, both trusted contrib extensions |
| Vue | 3.5.44 | |
| Vite | 8.3.4 | |
| Tailwind CSS | 4.3.3 | via `@tailwindcss/vite` |
| Vue Router | 5.4.0 | |
| Pinia | 4.1.0 | |
| TanStack Query (Vue) | 5.104.3 | |
| VueUse | 15.0.0 | |
| TypeScript | 6.0.3 | TypeScript 7.0 (native compiler) is the npm `latest` tag, but it does not ship the JavaScript compiler API that `vue-tsc` and `openapi-typescript` depend on. 6.0.3 is the newest release compatible with both. |
| vue-tsc | 3.3.13 | Verified against TypeScript 6.0.3 |
| Vitest | 5.0.5 | |
| Playwright | 1.64.1 | Uses the pre-installed Chromium |
| zod | 4.6.7 | Runtime message validation in the extension |

## Architecture decisions

1. **Single Go module at the repository root.** `db/migrations` must be embedded in the binary,
   and Go's `embed` cannot reach outside the module. All Go packages live under `services/` and `db/`.
   Shared code is in `services/internal/`, so both `services/api` and `services/worker` can import it.
2. **Embedded worker by default.** The API process runs the job runner in-process
   (`REA_WORKER_MODE=embedded`). `services/worker` runs the same runner standalone for operators who
   want to scale analysis and AI jobs separately. Jobs are claimed with `FOR UPDATE SKIP LOCKED`,
   so both modes can run concurrently safely.
3. **CRM is organization-wide; analysis projects belong to workspaces.** This is single-organization
   software. Workspaces group analysis projects (for example "Product sourcing" or "Client X audit").
4. **Configurable concepts** are provided by pipelines with custom stages (leads, opportunities,
   supplier qualification), custom field definitions, tags, saved filters and renameable entity labels.
5. **Polymorphic children** (notes, tasks, tags, attachments) reference records by
   `(entity_type, entity_id)`. Database triggers clean them up when the parent record is deleted.
6. **Extension authentication** uses a one-time pairing code issued from the web UI, then short-lived
   access tokens plus rotating refresh tokens scoped to the extension. Tokens are stored in the
   extension origin's IndexedDB, which content scripts and pages cannot read.
7. **Least-privilege extension.** No static content scripts or host permissions. The extension uses
   `activeTab` plus `scripting` to inject its extractor only after a user gesture, and requests optional
   host access for the configured server origin.
8. **AI is optional and provider-independent.** Every AI call goes through a provider interface.
   External providers need an administrator's explicit data-sharing acknowledgement and a per-request
   confirmation. Outputs cite numbered evidence items and are validated before they are stored.
9. **Evidence classification.** Every finding is `verified`, `inferred` or `unknown`, with a confidence
   score, evidence excerpt, locator and assumptions.

## Phases

See the specification. Each phase ends with a commit and a summary in `docs/progress.md`.
