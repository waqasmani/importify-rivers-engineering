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

## Add an extension site adapter
1. Module in `apps/extension/src/content/extract/<site>.ts` returning `AdapterOutput` fields with
   per-field confidence and evidence (see `shopify.ts`, `woocommerce.ts`); register it in the
   `BUILTIN` map in `extract/index.ts`. `merge.ts` combines adapters by confidence.
2. Built-in adapters are switched on and prioritized by server rows: add a migration inserting a
   `source_adapters` row with `kind = 'builtin'`, the same `key`, domain patterns and priority.
3. Never read cookies, form values, hidden inputs or token-like meta tags; the content script
   must make no network requests (`scripts/check-dist.mjs` enforces parts of this).
4. Fixture HTML in `testdata/fixtures` + a case in `apps/extension/tests/adapters.spec.ts`.
   Simple sites need no code: an administrator adds a selector adapter under Settings → Source
   adapters, which the extension receives from `/extension/config`.

## Release / deploy
```bash
deploy/scripts/backup.sh && git pull
docker compose -f deploy/docker-compose.yml up -d --build --wait
deploy/scripts/verify-backup.sh deploy/backups/<latest>
```
