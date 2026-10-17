---
name: rea-api
description: Use a self-hosted REA Importify server from Claude — create authorized analysis projects, add sources (OpenAPI specs, HAR/API examples, documents, screenshots, repository archives, HTML, text, workflows, captured pages), wait for analyzers, read verified/inferred/unknown findings, generate cited reports (API inventory, architecture, data model, technical docs, reconstruction plan), search the CRM and check or import products. Use when the user asks to analyze or reverse-engineer material with REA, pull REA findings or reports, or push data into their REA instance.
---

# Working with a REA Importify server

`scripts/rea.py` is a dependency-free Python CLI for the REA REST API. It needs two environment
variables:

```bash
export REA_URL=https://rea.example.com          # the server's public URL
export REA_TOKEN=reap_...                       # Profile → API tokens in the REA dashboard
python3 scripts/rea.py whoami
```

Create the token with only the scopes the task needs (for analysis: `analysis.read`,
`analysis.write`; for CRM lookups: `crm.read`; for product imports: `products.read`,
`products.write`). Ask the user for the URL and token; never print the token back.

## Ground rules (REA enforces them; follow them in what you do too)

- Only analyze material the user owns or is authorized to analyze. Before `project-create`, ask
  which basis applies (`owned`, `client_authorized`, `public_information`, `licensed`,
  `open_source`, `other`) and pass `--i-am-authorized` only after the user confirms.
- Never upload credentials, cookies, tokens or private keys as sources. Strip them from HAR
  files and configs first; REA strips recorded auth headers too, but do not rely on that.
- Conclusions about a public website or a screenshot cover only visible behaviour. When you
  summarize findings, keep **verified**, **inferred** and **unknown** apart and cite the finding
  IDs (`[F-xxxxxx]`) REA puts in reports.
- REA never publishes products, purchases or sends messages; neither should you on its behalf.

## Typical analysis

```bash
R=scripts/rea.py
python3 $R project-create --name "Inventory API review" --objective "Document the API" \
  --types api,document --basis client_authorized --note "SOW 2026-14" --i-am-authorized
P=<project id from the output>
python3 $R source-upload $P --kind openapi openapi.yaml
python3 $R source-upload $P --kind api_examples traffic.har.json
python3 $R source-upload $P --kind repository_archive src.zip
python3 $R source-add $P --kind text --title "Requirements" --content-file requirements.md
python3 $R runs $P --wait                      # exit status 2 if any analyzer failed
python3 $R findings $P --classification verified
python3 $R findings $P --classification unknown   # what the material cannot answer
python3 $R report $P --kind api_inventory --out api-inventory.md
```

Report kinds: `summary`, `technical_documentation`, `architecture`, `api_inventory`,
`data_model`, `workflow`, `reconstruction_plan`. Reports are deterministic (no AI) and cite their
findings. Upload kinds: `repository_archive` (zip/tar.gz; never executed), `document`
(PDF/DOCX/Markdown/text), `screenshot`, `openapi`, `api_examples`, `html`. JSON source kinds:
`text`, `html`, `openapi`, `api_examples`, `workflow` (`--steps-file` with
`[{"actor","action","system","data":[],"decision","outcome"}]`), `url` (`--fetch` only works when
the server enables fetching), `repository_path` (only allow-listed server paths),
`captured_page` (from the browser extension).

## Other commands

```bash
python3 $R search "acme"                          # CRM, products, findings the token can see
python3 $R get "/suppliers?q=acme&limit=20"       # any GET endpoint
python3 $R post /companies company.json           # any POST endpoint with a JSON body
python3 $R product-check payload.json             # normalization, warnings, duplicate candidates
python3 $R product-import payload.json --decision create_new
```

A product payload looks like
`{"url": "...", "title": "...", "price": "19.90", "currency": "EUR", "sku": "...", "brand": "...", "images": [{"url": "..."}], "specifications": {"Material": "Steel"}}`.
Imports land in REA's review queue; a person approves them in the dashboard.

The full API is described by `GET $REA_URL/api/v1/openapi.json`. Errors come back as
`{"error": {"code", "message", "fields"}}`; the CLI prints them and exits 1.
