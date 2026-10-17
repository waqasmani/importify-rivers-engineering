# Architecture reference

## Request path
Caddy (static dashboard, TLS) → `/api/*` → `rea-api`. Middleware order: request ID → client IP
(`X-Forwarded-For` only from `REA_TRUSTED_PROXIES`) → security headers → CORS → rate limit →
authenticate (session cookie + CSRF, personal token `reap_…`, extension token `reaa_…`,
integration key `reai_…`) → route permission (`s.requirePerm(auth.PermX)`) → handler.
Integration keys (organization-owned, scopes in `auth.IntegrationScopes`) are refused outside
`integrationRouteAllowed` (catalog GETs and product import submission) in `requireAuth`;
`Principal.Integration` is set and `Principal.User` is zero for them, so use `userIDPtr(r)`.

Handlers return `error`; `s.h()` converts `*APIError` to `{"error":{"code","message","fields"}}`.
Helpers: `decode` (strict JSON, 415 on wrong content type), `pathUUID`, `pagination`, `q*`
query helpers, `ok`, `created`, `noContent`, `errField`, `errNotFound`, `errConflict`.
Multi-write operations use `s.db.Tx(ctx, func(q *store.Queries, tx pgx.Tx) error {...})` and
respond only after the transaction commits. Audit with `s.audit(ctx, q, r, auditEvent{...})`.

## Data
PostgreSQL 18 with `pg_trgm` and `citext`. Polymorphic children (notes, record_tags, attachments,
tasks) use `(entity_type, entity_id)` with cleanup triggers; allowed types are the `entities` map
in `httpapi/crm_common.go`. Settings live in `app_settings` behind the `settingSpecs` registry.

## Catalog change capture
Triggers on products, product_variants, product_images, record_tags, tags (name) and suppliers
(name) write `catalog_change_events` in the writing transaction, so no write path needs code for
it. `catalog.Sequence` moves committed events into `catalog_changes` (the feed) under an advisory
lock and classifies them; `catalog.Load` maps rows to the API representation with an explicit
column list (`toProduct`, `catalogPricing`). New product columns stay private until mapped there;
non-catalog columns go into `rea_catalog_ignored_fields()`, selling-price columns into
`priceFields`. Webhooks (`internal/webhooks`) fan the feed out per endpoint in the worker.

## Jobs
`analysis_runs` and `ai_jobs` are claimed with `FOR UPDATE SKIP LOCKED` by the embedded worker
(`REA_WORKER_MODE=embedded`) or `rea-worker`. `Worker.RunOnce`/`Drain` run jobs synchronously in
tests.

## Analysis
Sources (url, captured_page, html, repository_archive, repository_path, openapi, api_examples,
document, screenshot, text, workflow) require `authorization_confirmed` + `authorization_basis`
on the project. Analyzers (`website`, `codebase`, `openapi`, `api_examples`, `document`,
`screenshot`, `workflow`) register in `analysis/types.go` and emit findings with
`classification` verified|inferred|unknown, `confidence` 0–1, `evidence`, `evidence_locator`,
`assumptions`, `origin` rule|ai|manual. Reports (`analysis.ReportKinds`) cite findings as
`[F-xxxxxx]` with an evidence index.

## AI
`ai.Provider` adapters: Ollama, OpenAI-compatible, OpenAI, Anthropic (official Go SDK). Tools in
`ai/tools.go` declare entity types, JSON output schema and apply mode
(summary|product_fields|tags|findings|report|followup|""). Evidence is numbered E1…En; `Ground`
strips invented citations and counts uncited claims. External providers need
`allow_external_data` plus per-request `confirm_external` (409 `external_confirmation_required`
otherwise). Results change data only through `POST /ai/jobs/{id}/apply`.

## Dashboard
Vue 3 + TS strict + Vite + Tailwind 4 + Vue Router + Pinia + TanStack Query + VueUse. API
client `@/api/client` (`api.get/post/...`, `ApiError`, CSRF header). Lists via
`useListQuery` + `DataTable` + `UiPagination`; records use `RecordPanels`, `TagEditor`,
`CustomFieldsEditor`; AI via `AiToolsPanel`/`AiJobResult`; Markdown via `MarkdownView`
(DOMPurify, Mermaid strict). Routes and permissions in `src/router/index.ts`.

## Extension
MV3 with pinned key (ID `olceibeplclcegianhjffambadoakkno`, public key in
`apps/extension/manifest-key.txt`), permissions activeTab, scripting, storage, contextMenus,
sidePanel; optional host permissions per site. `src/background` (service worker) holds tokens and
talks to the API; `src/content` is injected on demand (adapters in `extract/`, `picker.ts`,
`snapshot.ts`) and never sees credentials; `src/ui` is the Vue side panel/popup; `src/shared`
holds zod message schemas. Build with `scripts/build.mjs`, verified by `scripts/check-dist.mjs`.
