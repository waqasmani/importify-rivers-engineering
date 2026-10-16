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
