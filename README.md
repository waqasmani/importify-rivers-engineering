# REA Importify

**REA — Reverse Engineering Anything**, an integrated **Importify CRM**, and the **REA browser
extension**: a self-hosted platform for researching websites, products, APIs, documents,
workflows and authorized codebases, turning what you learn into evidence-backed findings and
reports, and running supplier and product sourcing in a CRM.

Everything runs on your own server. There is no vendor account, licence server, billing or
telemetry, and AI is optional.

## What it does

- **Analysis engine** — projects with sources (websites, captured pages, HTML, ZIP repositories,
  allow-listed repository paths, OpenAPI specs, recorded API examples, PDF/DOCX/Markdown
  documents, screenshots, text, workflows). Deterministic analyzers produce findings classified
  as **verified**, **inferred** or **unknown**, each with confidence, evidence, a locator and
  assumptions, and reports (technical documentation, architecture, API inventory, data model,
  workflow diagrams, reconstruction plans) that cite their evidence.
- **Importify CRM** — companies, contacts, suppliers, leads, opportunities with product lines,
  configurable pipelines and labels, tasks and reminders, notes and comments, attachments, tags,
  custom fields, saved filters, duplicate detection and merge, bulk actions, CSV import with dry
  run, CSV/JSON export, global search, activity timelines and a full audit log.
- **Product import manager** — capture → detect → extract → normalize → de-duplicate → review →
  save → link supplier. Products are never published, purchased or messaged about automatically.
- **Marketplace support** — declarative definitions for 31 marketplaces (AliExpress, Alibaba,
  1688, Taobao, Tmall, JD.com, DHgate, CJdropshipping, Banggood, Geekbuying, Tomtop, SaleYee, Temu,
  SHEIN, Rosegal, Wish, Walmart, Etsy, Kohl's, Costco, Bed Bath & Beyond, Bonanza, LTD Commodities,
  AllStarHealth, Flipkart, Snapdeal, Lazada, Shopee, Amazon, eBay, Nordstrom) with their regional
  domains: embedded page data, JSON-LD and selectors mapped to fields and variants without running
  page code. They have so far been tested only against synthetic fixtures; the **Adapter lab**
  checks them, or your own rule documents, against product pages you saved
  ([docs/marketplaces.md](docs/marketplaces.md)).
- **Browser extension** (Chrome, Manifest V3) — capture pages, import products with site adapters
  (JSON-LD, microdata, Open Graph, Shopify, WooCommerce, Alibaba, marketplace definitions and other
  rules adapters, custom selectors, generic fallback), an element picker, page inspector, quick
  research, AI analysis and save-to-CRM, paired to your server with short-lived, revocable tokens.
- **Optional AI** — Ollama, OpenAI-compatible servers, OpenAI or Anthropic for summaries,
  normalization, extraction, tagging, duplicate review, comparisons, documentation and plans.
  Outputs are schema-validated, grounded in numbered evidence, labelled as AI-generated and only
  applied when a person approves. External providers need explicit administrator and per-request
  consent.

## Quick start (Linux, Docker)

```bash
git clone https://github.com/waqasmani/importify-rivers-engineering.git rea
cd rea
deploy/scripts/init-env.sh                      # writes deploy/.env with generated secrets
docker compose -f deploy/docker-compose.yml up -d --build --wait
grep '^REA_SETUP_TOKEN=' deploy/.env | cut -d= -f2
```

Open http://localhost, paste the setup token and create the owner account. For a public server
with automatic HTTPS run `deploy/scripts/init-env.sh rea.example.com` instead. Full instructions,
including Docker installation, HTTPS options, backups, restore, upgrades and running without
Docker: **[docs/deployment.md](docs/deployment.md)**.

Install the extension: `pnpm install && pnpm --filter @rea/extension build`, then load
`apps/extension/dist` in `chrome://extensions` (Developer mode → Load unpacked) and pair it from
**Settings → Browser extension**. Details: [docs/extension.md](docs/extension.md).

## Stack

Go 1.27 (chi, pgx, sqlc, goose, slog) · PostgreSQL 18 · Vue 3 + TypeScript + Vite + Tailwind CSS 4
+ Vue Router + Pinia + TanStack Query + VueUse · Chrome Manifest V3 · Caddy · Docker Compose ·
OpenAPI 3 generated from the Go types.

## Documentation

| Document | Contents |
| --- | --- |
| [deployment.md](docs/deployment.md) | Install, HTTPS, first-run setup, backups, restore, upgrades, offline use, troubleshooting |
| [configuration.md](docs/configuration.md) | Every environment variable |
| [security.md](docs/security.md) | Responsible-analysis rules and security controls, with where they are enforced |
| [architecture.md](docs/architecture.md) | Components, data model, jobs, analysis engine, AI layer |
| [api.md](docs/api.md) | Authentication, conventions, OpenAPI |
| [extension.md](docs/extension.md) | Building, installing and pairing the extension, permissions, troubleshooting |
| [marketplaces.md](docs/marketplaces.md) | Supported marketplaces, verification status, Adapter lab, rule document reference, updating definitions |
| [development.md](docs/development.md) | Local development, tests, code generation |
| [progress.md](docs/progress.md) | What was built in each phase and how it was verified |

## Development

```bash
make deps
make dev-api      # API on :8080 against a local PostgreSQL
make dev-web      # dashboard on :5173
make test         # Go, dashboard, extractor and extension tests (set REA_TEST_DATABASE_URL)
```

See [docs/development.md](docs/development.md).

## Responsible use

REA analyzes material you own or are authorized to analyze. It records your authorization for
every source, never extracts credentials, cookies or tokens from pages, does not access private
endpoints or bypass logins, paywalls or rate limits, never executes uploaded code, and keeps
verified evidence, inferences and unknowns visibly apart. Conclusions from a public website or a
screenshot are limited to what is visible.
