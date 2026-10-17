# Web app conventions (apps/web)

The dashboard is Vue 3 + TypeScript (strict) + Vite + Tailwind CSS v4 + Vue Router + Pinia + TanStack Query + VueUse.
It talks only to the REA API under `/api/v1` on the same origin, using the session cookie and CSRF header.

## Layout

| Path | Purpose |
| --- | --- |
| `src/api/client.ts` | `api.get/post/put/patch/del/upload/raw`, `download()`, `ApiError` (status, code, fields, body), `errorMessage()` |
| `src/stores/auth.ts` | `useAuth()`: `user`, `organization`, `can(perm)`, `load()`, `login()`, `setup()`, `logout()` |
| `src/stores/settings.ts` | `useSettings()`: organization settings and `label(type, plural)` for configurable CRM labels |
| `src/router/index.ts` | All routes. `meta.perm` / `meta.anyPerm` gate pages; guards redirect to `/setup`, `/login` or `/forbidden` |
| `src/layouts/AppLayout.vue` | Sidebar (filtered by permission), global search (Ctrl+K), reminders bell, user menu |
| `src/lib/entities.ts` | Record types, their routes and permissions; `entityLink(type, id)`, `entityLabel(type)` |
| `src/lib/format.ts` | Date, money, bytes and list helpers, `toLocalInput`/`fromLocalInput` for `datetime-local` inputs |
| `src/lib/options.ts` | Value lists mirrored from server validation (task statuses, custom field types, …) |
| `src/components/ui/*` | `UiButton`, `FormField`, `UiModal`, `UiTabs`, `UiBadge`, `DataTable`, `UiPagination`, `PageHeader`, `StateViews`, `ToastHost`, `ConfirmHost` |
| `src/components/record/*` | Shared record features: `EntityPicker`, `TagEditor`, `NotesPanel`, `TasksPanel`, `TaskFormModal`, `AttachmentsPanel`, `ActivityTimeline`, `ChangesView`, `CustomFieldsEditor`, `CsvImportDialog`, `ExportButton`, `RecordPanels` |
| `src/components/ai/*` | `AiToolsPanel` (run tools for a record), `AiJobResult` (render + apply), `CitationList` |
| `src/components/content/MarkdownView.vue` | Sanitized Markdown with Mermaid (strict) for reports |
| `src/components/marketplaces/*` | Marketplaces page helpers (`marketplaces.ts`), `RulesAdapterFormModal`, the Adapter lab (`lab.ts`, `LabResults`) |
| `src/composables/*` | `useListQuery`, `useToast`, `useConfirm`, `useFieldErrors`, `useAiJob` |
| `tests/unit/**.spec.ts` | Vitest + @vue/test-utils + happy-dom |
| `e2e/*.spec.ts` | Playwright against a real API |

## Rules

- Types come from `@rea/api-types` via `Schemas['Name']` (re-exported from `@/api/client`). Never hand-write API shapes that exist in the spec. Regenerate with `pnpm api-types` after changing Go types.
- Every page handles loading, empty and error states (`StateViews`, `DataTable` does this for lists).
- Lists keep filters, sort and paging in the URL with `useListQuery(key, path, filterKeys)`; pair it with `DataTable` and `UiPagination`.
- Forms show server validation next to fields: `const { errors, capture } = useFieldErrors()`, then `:error="errors.field"` on `FormField`.
- Destructive or irreversible actions always go through `useConfirm().confirm({ danger: true, ... })`.
- Hide actions the user cannot perform with `auth.can('perm')`; the server enforces the same permissions.
- CRM concept names come from `useSettings().label('company', true)` so renamed labels show everywhere.
- Records with notes/tasks/files/activity/AI use `RecordPanels`; tags use `TagEditor`; custom fields use `CustomFieldsEditor`.
- AI output is always shown with `AiJobResult`, which labels it as AI-generated and requires an explicit apply step.
- Nothing is published, purchased or sent from the UI. Product approval only changes REA's own review status.
- No `v-html` except through `MarkdownView` (DOMPurify). External links use `rel="noopener noreferrer nofollow"`.
- Accessibility: labelled inputs (`FormField` wires `for`/`id`), buttons with text or `aria-label`, keyboard reachable menus.
- Saved pages in the Adapter lab are parsed with `DOMParser` and read with `@rea/extractor` only: never insert their markup into the live document and never render their images or other resources (list URLs as links instead).
- No new dependencies without updating this document and the root lockfile. Workspace packages in use: `@rea/api-types` (API types) and `@rea/extractor` (product extraction, also used by the extension; it brings `zod`).
