# REA Importify browser extension

The extension (`apps/extension`, package `@rea/extension`) captures web pages, imports products into
REA's review queue, inspects pages and saves companies or suppliers to the CRM, using your self-hosted
REA server. It is a Chrome Manifest V3 extension written in TypeScript (strict) with a Vue 3 side
panel. The side panel is the main interface; the toolbar popup shows the same app and can open the
side panel.

The extension never publishes products, places orders or sends messages. Imports land in REA's import
queue as **draft** products that a person reviews in the web app.

## Building

Requires Node.js 22+ and pnpm 10 (see the root `package.json`).

```sh
pnpm install                              # once, from the repository root
pnpm --filter @rea/extension build        # type-checks, builds dist/ and verifies it
pnpm --filter @rea/extension typecheck
pnpm --filter @rea/extension test         # Vitest unit tests (happy-dom)
pnpm --filter @rea/extension test:e2e     # Playwright (see "Testing" below)
```

`scripts/build.mjs` uses Vite's JavaScript API to produce `apps/extension/dist/`:

| File | Built from | Notes |
|---|---|---|
| `manifest.json` | `scripts/manifest.mjs` | Version from `package.json`, public key from `manifest-key.txt` |
| `background.js` | `src/background/` | Service worker, ES module (`"type": "module"`) |
| `content.js` | `src/content/` | Classic IIFE, injected on demand with `chrome.scripting.executeScript` |
| `sidepanel.html`, `popup.html`, `assets/` | `src/ui/` | One Vue 3 + Tailwind CSS app for both surfaces |
| `icons/icon-{16,32,48,128}.png` | `scripts/icons.mjs` | Generated locally, no downloads |

The build finishes by running `scripts/check-dist.mjs` (also runnable on its own with
`node scripts/check-dist.mjs`). It fails the build unless the manifest contains the expected public key
and extension ID, declares no `host_permissions`, has exactly the permissions and optional host
permissions listed below and the strict CSP, declares no `externally_connectable`, content scripts or
web-accessible resources, every referenced file exists, and no source maps, private keys, cookie access
or network calls in the content script are shipped.

## Installing

**Load unpacked (development and self-hosted use).** Open `chrome://extensions`, turn on *Developer
mode*, choose *Load unpacked* and select `apps/extension/dist`. After rebuilding, press the reload
button on the extension card.

**Packing a zip.** For distribution (Chrome Web Store upload, or an internal download that users load
unpacked) zip the contents of `dist/`:

```sh
cd apps/extension/dist && zip -r ../rea-importify-$(node -p "require('../package.json').version").zip .
```

Do not create `.crx` files from the repository: packing a `.crx` needs the private key, which is never
stored in the repository (`*.pem` and `*.key` are git-ignored; only the public key is committed).

## Extension ID and `REA_ALLOWED_ORIGINS`

`apps/extension/manifest-key.txt` holds the extension's **public** key (base64 DER, one line). Chrome
derives the extension ID from it, so every build, packed or unpacked, has the ID
**`olceibeplclcegianhjffambadoakkno`**. The REA server always allows the origin
`chrome-extension://olceibeplclcegianhjffambadoakkno` to make bearer-token API requests (CORS without
credentials), so no server configuration is needed for the standard build.

If you build the extension with **your own key** (or remove the `key`, which makes Chrome assign a
random ID to an unpacked extension), the ID changes and the server must allow it:

```sh
REA_ALLOWED_ORIGINS=chrome-extension://<your-extension-id>
```

`REA_ALLOWED_ORIGINS` is a comma-separated list. Entries starting with `chrome-extension://` are
treated as extension origins: they may call the API with extension bearer tokens but never with cookies.
Other entries (`https://…`) are extra web origins allowed to use the session cookie. Also update
`EXPECTED_ID` in `scripts/check-dist.mjs` for a custom-key build. The extension's own ID is shown under
*Settings → Permissions* in the side panel.

## Pairing

1. In the REA web app open **Settings → Browser extension** and create a pairing code. Codes are
   8 characters (shown as `ABCD-EFGH`), single-use and valid for 10 minutes. Your role needs the
   `extension.pair` permission.
2. Open the extension (toolbar button, then the side-panel button), go to **Server**, enter the server
   address shown next to the code, the code and a device name, and choose **Pair extension**.
3. The service worker calls `POST /api/v1/extension/pair` and receives a token pair: an access token
   (`reaa_…`, valid 1 hour) and a refresh token (`rear_…`, valid 30 days), plus the device ID.

The extension then loads `GET /api/v1/extension/config` (user, effective permissions, enabled source
adapters, capture and import settings, whether AI is enabled). The effective permissions are the
intersection of the extension scope (`crm.read`, `crm.write`, `products.read`, `products.write`,
`captures.write`, `analysis.read`, `analysis.write`, `ai.use`) and your role, so changing your role in
REA changes what the extension can do. Actions you are not allowed to perform are hidden or disabled.

### How credentials are kept

- Every REA API call is made by the **service worker**. Pages, the popup and the side panel send it
  validated messages; content scripts never receive tokens or the server address.
- The refresh token, device ID and server address are stored in **IndexedDB in the extension's
  origin**. The short-lived access token is in **`chrome.storage.session`** restricted to trusted
  extension contexts (memory only, cleared when the browser closes). Nothing is written to
  `chrome.storage.local`, which content scripts could read.
- Requests use `Authorization: Bearer reaa_…` with `credentials: 'omit'`; cookies are never sent.
- The access token is refreshed with `POST /api/v1/extension/token/refresh` shortly before it expires
  (and once more if the server rejects it). Each refresh **rotates** the refresh token; concurrent
  callers share a single refresh so a refresh token is never presented twice.

## Permissions and why each is needed

| Permission | Why |
|---|---|
| `activeTab` | Read the current tab only after you click the REA toolbar button or the *Capture with REA* menu item. |
| `scripting` | Inject the page reader and element picker into that tab when you ask for it. Nothing is injected automatically. |
| `storage` | Keep the pairing, the short-lived access token and preferences in the extension's protected storage. |
| `contextMenus` | Add *Capture with REA* to the right-click menu (page, selection, image). |
| `sidePanel` | Show REA in the browser side panel. |
| `optional_host_permissions` `https://*/*`, `http://*/*` | Requested **per site**, only when you turn on *Allow on this site* (Settings), so REA can read that site later without another click. Revoke any site in *Settings → Site access*. |

There are no `host_permissions`, no static content scripts, no `externally_connectable` (web pages
cannot message the extension), no remote code and a strict extension CSP
(`script-src 'self'; object-src 'none'; …`). API requests to the REA server rely on the server's CORS
allowance for the extension origin rather than on host permissions.

## Local (http) and HTTPS servers, and CORS

The server address must use **HTTPS**. Plain `http://` is accepted only for servers on the same
computer: `http://localhost`, `http://127.0.0.1` and `http://[::1]` (any port), for local development.
The address is normalized: a missing scheme becomes `https://` (or `http://` for those local hosts),
trailing slashes and a pasted `/api/v1` suffix are removed, and user names, passwords, queries and
fragments are rejected or dropped. A server under a path prefix (`https://example.com/rea`) works.

The browser enforces CORS on every API call. The server answers requests from the extension origin with
`Access-Control-Allow-Origin` and no credentials (see `services/internal/httpapi/middleware.go`). If the
origin is not allowed, the browser blocks the response and the extension reports a CORS problem
instead of a generic network error (it probes `/healthz` with a `no-cors` request to tell the two apart).

## Revocation

- **Disconnect** (Server section) calls `POST /api/v1/extension/disconnect`, which revokes the device and
  its tokens on the server, then wipes every local credential. If the server cannot be reached, local
  credentials are still removed and the extension tells you to revoke the device in the web app.
- **Revoking the device in REA** (*Settings → Browser extension*) takes effect on the extension's next
  request: the refresh fails with 401, the extension wipes its credentials and shows why.
- A **reused refresh token** (for example a copied token presented after it was rotated) makes the
  server revoke the device; the extension then also wipes its credentials and asks you to pair again.
- **Clear local data** (Settings) disconnects, then deletes credentials, preferences, picked selections
  and cached settings.

## What is captured, and what never is

Reading a page happens **locally**. The Capture, Import and Inspect views show what was found; nothing is
sent to REA until you save a capture, check or submit an import, or run an analysis.

When you save a capture (`POST /api/v1/captures`) REA receives: the page URL (with token-like query
parameters removed), canonical URL, title, meta description, detected page type, the structured data
found (JSON-LD, microdata, Open Graph and non-sensitive meta tags), the extracted product fields with
per-field confidence and evidence, a page outline (platform, technologies, headings, link counts), the
selections you picked, warnings and, only if allowed, an HTML snapshot.

Never captured or sent:

- cookies (`document.cookie` is never read), passwords, access tokens, session IDs;
- values of form fields: inputs, text areas and selects are skipped when reading text; password and
  hidden inputs are dropped from snapshots;
- meta tags, attributes, JSON-LD keys and URL parameters whose names suggest tokens, CSRF values,
  nonces, sessions, secrets or API keys;
- inline or external scripts. Scripts are only parsed as data: JSON-LD, Shopify product JSON and
  the embedded state that marketplace definitions name (for example `window.runParams` or
  `__NEXT_DATA__`), read with a brace scanner and a JSON/object-literal parser, never evaluated.
  Token-like keys (CSRF values, sessions, tokens) are removed before anything reads that data,
  nothing on the page is executed and the content script makes no network requests.

**HTML snapshots** are optional. They are taken only when the server's capture setting
`store_html_snapshots` is on **and** you enable them in *Settings → HTML snapshots*; your size limit
cannot exceed the server's `max_snapshot_kb`. The snapshot is built from a detached copy of the page
and sanitized before it leaves the page: scripts, `noscript`, frames, objects, templates, comments,
`base`, event-handler attributes, `javascript:` URLs, `srcdoc`, form values, password and hidden
inputs and token-like attributes and meta tags are removed. Snapshots over the limit are slimmed
(styles and SVG removed) or left out with a warning. The server sanitizes again and stores the
snapshot as inert text.

## Features

- **Server** – pair, connection status, user, organization, server version, permissions, disconnect.
- **Capture** – read the page (page type detection, platform, adapters), element picker, selections,
  save the capture. The picker highlights elements inside a closed shadow-root overlay; click an element
  to assign it to *Title*, *Price*, *Image*, *Description*, *Specifications table* or *Custom note*;
  press **Esc** to finish. Only real (trusted) clicks count and the page's own click handlers are blocked
  while picking. Right-click *Capture with REA* on a page, selection or image opens the side panel with
  that context (selected text or the image is added as a selection).
- **Import** – editable product form with per-field confidence and evidence, **Check with REA**
  (`POST /api/v1/imports/products/check`: normalized values, warnings, duplicates, suggested supplier),
  decision (create, update an existing product, let REA decide, skip), supplier choice, tags, then
  **Send to review queue** (`POST /api/v1/imports/products`) with links to the import and the queue.
- **Inspect** – local-only view of platform, technologies (generator tags, asset hosts, page markers),
  canonical URL, headings, JSON-LD, microdata, Open Graph and meta tags.
- **Research** – searches REA (`GET /api/v1/search`) for the page's domain, site name, brand or supplier.
- **Analysis** – creates an analysis project with the captured page as a source (authorization basis
  and confirmation required). When AI is enabled, runs the `summarize` tool on the capture
  (`POST /api/v1/ai/jobs`). If the provider is outside your infrastructure, REA answers
  `409 external_confirmation_required`; the extension shows the provider, host, model and data
  categories and only sends after you confirm. Results are labelled AI-generated.
- **CRM** – checks existing companies and suppliers by name and domain, creates a company or supplier,
  then optionally a contact and a lead.
- **Recent** – captures made from this browser (`GET /api/v1/captures?device_id=…`).
- **Settings** – *Allow on this site*, granted sites with revoke, snapshot preference, permissions,
  clear local data.

### Extraction adapters

Extraction lives in the shared package `packages/extractor` (`@rea/extractor`, DOM APIs only), which
the content script bundles and the dashboard's Adapter lab runs too. The content script adds only the
sanitized snapshot. The content script runs every enabled adapter that matches the page's domain (from
`/extension/config`; built-in defaults before pairing) in priority order and merges the results field
by field, keeping the most confident value and recording which adapter and selector or property
produced it (`field_confidence`, `evidence`, `extraction_method`, `adapter_key` in the import payload):
schema.org **JSON-LD** (Product, ProductGroup, Offer, AggregateOffer, Organization), **microdata**,
**Open Graph** / `product:` meta tags, **Shopify** (DOM markers and embedded product JSON; prices in
cents), **WooCommerce** (body classes, price markup, `variations_form` data), **Alibaba** (visible
details, tiered prices, supplier), **marketplace definitions and other rules adapters** (declarative
rule documents for 31 marketplaces such as AliExpress, Amazon, Temu or Lazada, see
[marketplaces.md](marketplaces.md); delivered by `/extension/config` only after pairing and run only
when their domain matches and their product signal holds), **server-defined selector adapters** (CSS
selectors configured in REA under *Settings → Source adapters*, matched by domain pattern) and
**generic** heuristics (headings, price patterns outside struck-through prices, prominent images,
specification tables, SKU patterns). A marketplace definition outranks the generic adapters, which
fill only the fields it leaves empty. List price, shipping and rating counts that definitions find
are kept with the capture as offer metadata (`extracted.metadata`), never imported as product
fields. When no price is found the payload says so in its warnings; no price is ever guessed.

## Testing

- `pnpm --filter @rea/extractor test` runs the extraction tests in `packages/extractor/tests`: every
  adapter against the fixtures in `testdata/fixtures`, the rules engine (brace scanner, path language,
  transforms, variants, limits, malformed data) and every marketplace definition against its fixture
  in `testdata/fixtures/marketplaces`.
- `pnpm --filter @rea/extension test` runs the unit tests in `apps/extension/tests`: the content
  script's extraction entry with snapshots and rules adapters, the snapshot sanitizer, message schemas
  and sender checks, the element picker, server URL validation, and the token
  pairing/refresh/rotation/401-wipe logic with a mocked `fetch`.
- `pnpm --filter @rea/extension test:e2e` (after a build) runs Playwright with Chromium
  (`REA_E2E_CHROMIUM`, else `/opt/pw-browsers/chromium` when present, else Playwright's own
  download), loading `dist/` with
  `--load-extension` in headless mode. `e2e/load.spec.ts` checks the extension ID, the side panel and
  popup, message validation and that the shipped build cannot read a page without a grant.
  `e2e/pairing-capture.spec.ts` needs a REA API. By default the Playwright config starts a throw-away
  one on port 18105 with a fresh `rea_ext_e2e` database (`deploy/scripts/e2e-api.sh`; set
  `REA_API_BIN` to a built `rea-api` to skip `go run`); set `REA_E2E_API_URL` to use an existing
  server instead. It signs in (or runs first-time setup with
  `REA_E2E_SETUP_TOKEN`, default `dev-setup-token`), serves the fixtures on `127.0.0.1:18115`, pairs from
  the side panel page, picks the specification table, captures with a snapshot, imports, verifies the
  records through the API, creates an analysis project, exercises the external-AI confirmation and
  checks disconnect and server-side revocation. Optional variables: `REA_E2E_EMAIL`,
  `REA_E2E_PASSWORD`, `REA_E2E_PAIRING_CODE`, `REA_E2E_FIXTURE_PORT`, `REA_E2E_API_PORT` and
  `REA_E2E_DB` (database of the throw-away API).
  Chromium cannot accept the optional host-permission prompt under automation, so this spec loads a
  temporary copy of `dist/` whose manifest additionally grants the fixture origin only; the shipped
  build is unchanged.
  `e2e/marketplace-capture.spec.ts` pairs again and captures the synthetic AliExpress fixture under
  `https://www.aliexpress.com/…` with the shipped definition, then imports it with its variants. It
  runs offline: Chromium starts without a proxy and cannot resolve any host but the local API,
  Playwright answers the page request with the fixture and aborts every other request to the
  marketplace, and the test asserts that the page was the only marketplace request.

## Troubleshooting

**"The extension's sign-in has expired or was revoked."** The refresh token expired (30 days without
use), the device was revoked in REA, or a rotated refresh token was reused. Local credentials have been
removed; create a new pairing code in REA and pair again.

**"The pairing code is invalid, already used or expired."** Codes work once and expire after 10
minutes. Create a new one. Make sure you are pairing with the server that issued it.

**"Can't reach the REA server at …"** Check the address (including `https://` and the port), your
network or VPN, and that the server is running (`/healthz` should answer). HTTPS is required except for
`localhost`, `127.0.0.1` and `[::1]`; certificate errors also appear as unreachable servers, so open the
server address in a tab to check the certificate.

**"The REA server … is reachable but did not allow requests from this extension."** This is a CORS
rejection. The standard build's ID is always allowed; a build with a different key needs its origin in
`REA_ALLOWED_ORIGINS` (see above). Restart the API after changing the setting. A reverse proxy must pass
the `Origin` header through and must not add its own CORS headers.

**"The server … answered with a redirect."** Use the exact address of the REA server. A proxy that
redirects `http://` to `https://` or adds a trailing path is not followed, because bearer tokens are
never sent across redirects.

**"REA does not have access to this page yet."** The extension only reads a tab after a user gesture.
Click the REA toolbar button while the page is open, use *Capture with REA* from the right-click menu, or
turn on *Allow on this site* under Settings. Access granted by a click ends when the tab navigates.
Browser pages (`chrome://`, the Chrome Web Store, other extensions) and local files cannot be captured.

**Snapshots are never included.** The server's capture setting has snapshots off, or your preference
is off, or the sanitized page is larger than the size limit (the capture's warnings say so).

**AI analysis is missing or fails.** The AI section is hidden when AI is switched off on the server or
your role lacks `ai.use`. "No AI provider is configured" means an administrator must add one. External
providers additionally need the administrator's data-sharing permission and your confirmation for each
request.
