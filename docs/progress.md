# Progress log

## Phase 1 — Repository assessment
- The repository held only a README. Toolchain versions were verified and recorded in `docs/implementation-plan.md`.

## Phase 2 — Foundation
- Files: `go.mod`, `sqlc.yaml`, `db/migrations/0000{1..4}_*.sql`, `db/queries/*.sql`, `services/internal/{config,store,migrate,auth,secretbox,normalize,valid,ratelimit,filestore,httpapi,worker,app,testutil}`, `services/api/main.go`, `services/worker/main.go`.
- Done: full schema for all modules, embedded goose migrations, sqlc queries, argon2id passwords, cookie sessions with CSRF tokens, idle and absolute session expiry, account lockout, login rate limiting, roles and permissions, first-run setup with a one-time token written to the data directory, users and roles administration, ownership transfer, personal API tokens, settings registry, workspaces, audit log, extension pairing with rotating refresh tokens and reuse detection, CORS, security headers, structured logging, graceful shutdown, health and readiness endpoints, job worker skeleton.
- Tests: `go test ./services/...` with `REA_TEST_DATABASE_URL` set — setup, login, CSRF, content-type enforcement, CORS, permissions, lockout and rate limiting, extension pairing, refresh rotation, reuse detection and revocation, audit redaction, settings validation, password hashing, tokens, encryption, normalization.

## Phase 3 — CRM backend
- Files: `services/internal/httpapi/{crm_common,companies,contacts,suppliers,deals,activity,csvio,search}.go`, `db/queries/{companies,contacts,suppliers,leads,opportunities,pipeline,activity,search}.sql`.
- Done: companies, contacts, suppliers, leads, opportunities with line items, configurable pipelines and stages, lead conversion, stage-driven opportunity status, tasks with reminders and overdue state, notes and threaded comments, attachments with safe download headers, tags, custom field definitions with typed validation, saved filters, duplicate detection, record merging that moves related records, bulk actions, CSV and JSON export with formula-injection protection, CSV import with column mapping, dry run and per-row errors, global search filtered by permission, dashboard metrics and activity timelines.
- Tests: `services/internal/httpapi/crm_test.go`.

## Phase 4 — Product import backend
- Files: `services/internal/products/`, `services/internal/htmlx/`, `services/internal/httpapi/{products,imports}.go`, `db/queries/{products,imports}.sql`.
- Done: payload normalization (canonical URLs, price and currency parsing, GTIN check digits, availability, images, variants, specifications, confidence, evidence), duplicate scoring by URL, GTIN, store SKU, brand plus MPN and title similarity, import queue with explicit create, update or skip decisions, refresh of existing products from new captures, supplier auto-linking by domain, review workflow (approval only activates the product internally), variants, images, supplier links, merge, bulk, CSV product import, captured pages with sanitized optional snapshots, selector-based source adapters.
- Tests: `services/internal/products/payload_test.go`, `services/internal/htmlx/sanitize_test.go`, `services/internal/httpapi/products_test.go`.

## Phase 6 — REA analysis engine
- Files: `services/internal/analysis/`, `services/internal/safearchive/`, `services/internal/safefetch/`, `services/internal/httpapi/analysis.go`, `services/internal/worker/{analysis,retention}.go`, `testdata/fixtures/`.
- Done: analysis projects with mandatory authorization confirmation and basis, sources (URL with optional SSRF-safe fetching that honours robots.txt, captured pages, HTML, ZIP/tar.gz repositories, allow-listed repository paths, OpenAPI, recorded API traffic with credentials stripped, PDF/DOCX/Markdown documents, screenshots, free text, structured workflows), queued analysis runs, analyzers for websites, codebases, OpenAPI, API examples, documents, screenshots and workflows, findings classified as verified, inferred or unknown with confidence, evidence, locators and assumptions, manual findings, deterministic reports (summary, technical documentation, architecture, API inventory, data model with ER diagram, workflows with Mermaid, reconstruction plan) with an evidence index, Markdown and JSON export, capture retention and orphan file cleanup.
- Security: archive path traversal, links and decompression bombs rejected; sensitive files never read; environment variable values never read; recorded Authorization headers, cookies and token-like fields removed before storage; private network addresses blocked at dial time.
- Tests: `services/internal/analysis/analysis_test.go`, `safearchive_test.go`, `safefetch_test.go`, `httpapi/analysis_test.go`.

## Phase 7 — AI orchestration
- Files: `services/internal/ai/`, `services/internal/httpapi/ai.go`, `services/internal/worker/ai.go`, `db/migrations/00005_ai_apply.sql`.
- Done: provider-independent interface with adapters for Ollama (`/api/chat` with JSON-schema format), OpenAI and OpenAI-compatible servers (`/chat/completions` with `json_schema`, falling back to JSON mode for older servers) and Anthropic (official Go SDK, structured outputs, explicit effort, server-side refusal fallbacks); encrypted API keys that are never returned or logged; local versus external endpoint detection; external providers need an administrator's data-sharing permission plus a per-request confirmation naming the data categories and host; thirteen tools (summarize, normalize product, extract structured fields, suggest tags, duplicate review, compare products, technical documentation, codebase explanation, data model, API integration plan, reconstruction plan, workflow diagram, supplier follow-up draft); numbered evidence with prompt-injection guidance; schema validation and grounding that removes invented citations and flags uncited claims; retries for transient failures; human-approved apply steps that save AI-generated reports, findings, tags, product fields or draft tasks, always labelled as generated. Nothing is ever sent, published or purchased.
- AI-disabled operation: with no provider, or with AI switched off in settings, AI endpoints return a clear 409 and every other feature works.
- Tests: `services/internal/ai/ai_test.go` (adapters against fake servers, including the Anthropic request shape and refusal handling), `services/internal/httpapi/ai_test.go`.

## Phase 5 — Browser extension
- Files: `apps/extension` (`src/background` service worker, `src/content` adapters/picker/sanitizer, `src/ui` Vue side panel and popup, `scripts/build.mjs`, `scripts/check-dist.mjs`), `docs/extension.md`.
- Done: Manifest V3 with a pinned public key (extension ID `olceibeplclcegianhjffambadoakkno`), permissions limited to activeTab, scripting, storage, contextMenus and sidePanel, per-site optional host access, strict CSP; pairing with one-time codes, short-lived access tokens and rotating refresh tokens kept in extension-only storage, automatic refresh, wipe on revocation; zod-validated messages with sender checks; on-demand content script with JSON-LD, microdata, Open Graph, Shopify, WooCommerce, Alibaba, server-defined selector and generic adapters merged with per-field confidence and evidence; element picker in a closed shadow root; snapshot sanitizer that drops scripts, handlers, frames and every form value; side panel sections for connection, capture, product import (check, duplicate decision, submit to the review queue), page inspector, quick research, analysis and AI summary with external-data confirmation, save to CRM, recent captures and site permissions.
- Tests: 99 Vitest tests (every shared fixture, the login page leaks no password, token or cookie, sanitizer, message schemas, token rotation and wipe, server URL rules) and 15 Playwright tests that load the built extension in Chromium, check the ID and isolation from web pages, then pair, capture, import, create an analysis project, confirm external AI, disconnect and observe server-side revocation against a throw-away API.

## Web dashboard
- Files: `apps/web` (router with permission guards, layout with global search and reminders, shared record panels, AI panel, Markdown/Mermaid viewer, and screens for the dashboard, CRM, products and imports, analysis, AI jobs, settings and administration), `docs/web-conventions.md`.
- Done: every page listed in the specification — sign-in and first-run setup, dashboard, global search, analysis projects, wizard with per-source authorization, results and evidence, codebase and API documentation viewers, report viewer, import queue, product detail and variants, supplier directory, company and contact management, pipelines board, tasks, activity history, extension setup and devices, AI provider settings, users and roles, workspaces, audit log, backups, export and system settings, profile with sessions and tokens.
- Tests: 25+ Vitest component test files and 10 Playwright end-to-end tests (`apps/web/e2e`) that run the built dashboard against a real API with a fresh database: setup, sign-in and redirects, security headers, CRM record with note, tag and search, tasks, product import with duplicate detection and review-only approval, analysis from OpenAPI to a deterministic report, viewer restrictions, configurable labels.
- Backend fixes found while building the screens: list and export endpoints failed for records without a contact or image (regression test added); built-in adapters could not be toggled; user search, admin e-mail changes and unlock; supplier company filter; pipeline totals for leads and suppliers; respecting "no supplier" on import; REA availability values; file listings for codebase sources; role names in user and extension responses.

## Phase 8 — Hardening and deployment
- Files: `Dockerfile` (api, web and extension targets), `.dockerignore`, `deploy/docker-compose.yml`, `deploy/proxy/Caddyfile`, `deploy/proxy/nginx.conf.example`, `deploy/systemd/rea-api.service`, `deploy/scripts/{init-env,backup,verify-backup,restore,smoke-test,dev-api,e2e-api}.sh`, `.env.example`, `Makefile`, `.github/workflows/ci.yml`, `docs/{deployment,configuration,security,architecture,api,development}.md`, `README.md`, `services/internal/migrate/migrate_test.go`.
- Done: pinned base images; API image runs as an unprivileged user on a read-only root filesystem with the PostgreSQL 18.6 client tools copied from the database image; Caddy serves the dashboard with security headers and proxies the API; only the web server publishes ports; backups (database dump, data directory, manifest, checksums, retention), verification in a temporary database, restore of full or database-only backups; migration lint (sequential, reversible, no destructive Up statements) and a full rollback/re-apply test; CI for Go, dashboard, extension, end-to-end and Docker smoke tests.

## Phase 9 — Final verification (2026-10-17)
Run from a fresh clone of the branch with `pnpm install --frozen-lockfile`:

| Check | Result |
| --- | --- |
| `make build` (API, worker, dashboard, extension with dist check) | passed |
| `make lint` (gofmt, go vet, migration lint, TypeScript) and `sqlc diff` | passed, generated code current |
| OpenAPI document and TypeScript types regenerated | no diff |
| Go tests with PostgreSQL (`REA_REQUIRE_DB_TESTS=1`) | 90 passed, 0 failed, 0 skipped |
| Dashboard component tests | 26 files, 96 tests passed |
| Extension unit tests | 6 files, 99 tests passed |
| Dashboard Playwright tests against a real API | 10 passed |
| Extension Playwright tests (load, pair, capture, import, revoke) | 15 passed |
| Docker images (`api`, `web`, `extension` targets) | built |
| `docker compose up --wait` + `deploy/scripts/smoke-test.sh` | passed: health, headers, setup, sign-in, write, backup, verify, restore, restored data checked |
| Container checks | API non-root, read-only root filesystem, only the web server publishes ports |
| Source scan | no TODO/FIXME, mocks, demo data, placeholder screens or committed secrets |

Not verified in this environment, and why:
- The GitHub Actions workflow was not executed (no runner here); its YAML parses and each step's commands were run locally.
- No real AI provider was called (no keys or outbound access); adapters are tested against fake servers that check request shapes, and the dashboard and extension handle the unconfigured and failing cases.
- Automatic HTTPS for a public domain (Let's Encrypt) was not exercised; the Caddyfile validates and plain HTTP on localhost was tested end to end.
- Browser menus (the extension's context menu, the popup's "Open side panel") and the per-site permission prompt cannot be driven by automation; the end-to-end test grants the fixture origin through a temporary manifest copy.
- Product extraction was tested on local fixture pages, not live third-party sites.
- Docker builds here needed the environment's proxy passed as build arguments and its CA as a build secret (`--secret id=extra_ca,…`), which the Dockerfile supports for corporate proxies.

Known limitations (documented in the UI):
- Tasks attach notes and activity to their linked record rather than having their own.
- Dismissing a duplicate suggestion lasts until the page reloads; merging is persistent.
- Personal API tokens cannot carry administrative scopes.
