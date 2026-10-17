# Recipes

## Add a database change
1. New file `db/migrations/0000N_short_name.sql` with `-- +goose Up` and a working
   `-- +goose Down`. Use `-- +goose StatementBegin/End` around functions.
2. Never drop or rewrite data in Up. If unavoidable, put `-- rea:allow-destructive <reason>` on the
   line before and explain it in the PR.
3. `go test ./services/internal/migrate` (lint + rollback/re-apply on a real database).

## Add a query
1. Write it in `db/queries/<area>.sql` with a `-- name: DoThing :one|:many|:exec|:execrows` header.
2. `sqlc generate` (v1.31.1). Nullable columns become pointers; `numeric` is `decimal.Decimal`.

## Add an API endpoint
1. Handler in `services/internal/httpapi/<area>.go`: decode → validate with `valid.New()` →
   permission checks (`s.can`, `s.requireEntity` for polymorphic records) → query/Tx → audit →
   respond. Never write the response inside a Tx closure.
2. Mount it in the area's `mount*` function with `r.With(s.requirePerm(auth.PermX))`.
3. Register it in `Operations()` in `openapi.go` with request/response types and query params.
4. Test in `<area>_test.go` with `newEnv(t)`; the client validates every response against the
   spec. Cover permission denial, validation errors and the happy path.
5. `make generate` and commit `packages/api-types` changes.

## Add a dashboard screen
1. Route in `apps/web/src/router/index.ts` with `meta.perm`; nav entry in `layouts/AppLayout.vue`.
2. View in `src/views/<area>/`. Use `Schemas['X']` types, `useListQuery`, `useFieldErrors`,
   `useConfirm` for destructive actions, `auth.can()` to hide forbidden actions, `StateViews`
   for loading/empty/error.
3. Component test in `tests/unit/<area>/` with `mountView` + `mockApi`.

## Add an analyzer
1. `services/internal/analysis/<name>.go`: type with `Name()` and
   `Analyze(ctx, Input) (Result, error)`, `func init() { register(x{}) }`.
2. Emit findings with honest classification: only `verified` for what the evidence shows;
   anything about hidden implementation is `inferred` with assumptions, or `unknown`.
3. Map the source kind to it in `DefaultAnalyzer`, add fixtures to `testdata/fixtures`, test it.

## Add an AI tool
1. Entry in `ai.Tools` with key, label, entity types, `schema` (use `obj/arr/str/cites` helpers so
   every provider's structured output accepts it), `task` text and apply mode.
2. Evidence gathering for new entity types in `worker/ai.go`; apply logic in
   `httpapi/ai.go handleApplyAIJob` if it writes data (always label output as AI-generated).
3. Render it in `apps/web/src/components/ai/AiJobResult.vue`.

## Add or fix a marketplace (rules adapter) — preferred for marketplaces
1. Write `db/marketplaces/<key>.json`: `key` (= file name), `name`, `region` (global, china,
   cross_border, north_america, south_asia, southeast_asia), `description`, `domains` (bare host and
   `*.host` per regional domain, max 100), `priority` (60), `version` (raise it on every change),
   `verification` (`status: "synthetic_fixture"` unless real pages were checked, a `note` describing
   the assumed layout, `fixture`, `fixture_url`) and `rules` (see docs/marketplaces.md: sources,
   field candidates, path language, transforms, `list`/`property_join` variants, product signal,
   `currency_by_host`). Never regexes or code; no `>` in selectors.
2. Write `testdata/fixtures/marketplaces/<key>.html`: a minimal synthetic page with invented data in
   that layout (never copy real pages, images or text), including token-like keys or hidden inputs
   containing `should-not-leak` to prove they are dropped.
3. Add expectations to `EXPECTED` and the key to `REQUIRED` in
   `packages/extractor/tests/marketplaces.spec.ts` (title, price + currency, images, variants/specs)
   and to `required` in `services/internal/marketplaces/definitions_test.go`.
4. `pnpm --filter @rea/extractor test`, `go test ./services/internal/marketplaces`; the API syncs the
   definition at start-up (existing installs keep enabled/priority). Verify on a saved real page in
   the Adapter lab before claiming anything beyond `synthetic_fixture`.
5. Changing the rule language: update `rules/schema.ts` and `services/internal/marketplaces/rules.go`
   (+ `path.go`) together and add cases to `testdata/rules/validation-cases.json`.

## Add an extension site adapter (code)
1. Module in `packages/extractor/src/adapters/<site>.ts` returning `AdapterOutput` fields with
   per-field confidence and evidence (see `shopify.ts`, `woocommerce.ts`); register it in the
   `BUILTIN` map in `packages/extractor/src/page.ts`. `merge.ts` combines adapters by confidence.
   Use only DOM APIs (the package typechecks without Node or Chrome types).
2. Built-in adapters are switched on and prioritized by server rows: add a migration inserting a
   `source_adapters` row with `kind = 'builtin'`, the same `key`, domain patterns and priority.
3. Never read cookies, form values, hidden inputs or token-like meta tags; the content script
   must make no network requests (`scripts/check-dist.mjs` enforces parts of this).
4. Fixture HTML in `testdata/fixtures` + a case in `packages/extractor/tests/adapters.spec.ts`.
   Simple sites need no code: an administrator adds a selector adapter under Settings → Source
   adapters, or a rules adapter under Products → Marketplaces (tested in the Adapter lab), which the
   extension receives from `/extension/config`.

## Release / deploy
```bash
deploy/scripts/backup.sh && git pull
docker compose -f deploy/docker-compose.yml up -d --build --wait
deploy/scripts/verify-backup.sh deploy/backups/<latest>
```
