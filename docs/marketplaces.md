# Marketplaces

REA reads product pages from 31 marketplaces through **marketplace definitions**: declarative
"rules" adapters that say where each marketplace keeps product data on its pages. The definitions
ship with REA in `db/marketplaces/<key>.json`; the extension receives the enabled ones from
`/extension/config` and runs them in the page you capture, next to the generic JSON-LD, microdata,
Open Graph and heuristic adapters. Products you import land in REA's review queue as drafts and are
consumed by other systems through REA's REST API.

> **Verification status.** Every shipped definition describes a marketplace's page layout as it is
> publicly understood and has been tested **only against a synthetic fixture**
> (`testdata/fixtures/marketplaces/<key>.html`): a minimal page with invented products that
> reproduces the assumed layout. None has been verified on live marketplace pages, and REA's
> developers did not fetch or scrape the marketplaces to write them. Marketplaces change their
> markup often. **Before relying on a definition, check it against real product pages** with the
> Adapter lab (a page you saved) or with the extension on a live page, and review every captured
> product before importing it. The dashboard shows each definition's status ("Synthetic fixture
> only") and the layout it assumes.

## Supported marketplaces

The *Reads* column lists the data sources a definition uses; *Fields* lists what it can fill (list
price, shipping, rating and rating count are offer metadata, see below). Every definition falls
back to the generic adapters for anything it does not read.

| Marketplace | Key | Region | Domains | Reads | Fields |
| --- | --- | --- | --- | --- | --- |
| Amazon | `amazon` | Global | amazon.com, amazon.co.uk, amazon.de, amazon.fr, amazon.it, amazon.es, amazon.ca, amazon.com.au, amazon.co.jp, amazon.in, amazon.ae, amazon.sa, amazon.com.mx, amazon.com.br, amazon.nl, amazon.se, amazon.pl, amazon.sg, amazon.com.tr, amazon.com.be, amazon.eg | attribute JSON, inline-script marker, script JSON, CSS selectors | title, description, brand, SKU, price, list price, availability, category, images, specifications, supplier, supplier URL, shipping, rating, rating count, variants (list) |
| eBay | `ebay` | Global | ebay.com, ebay.co.uk, ebay.de, ebay.fr, ebay.it, ebay.es, ebay.ca, ebay.com.au, ebay.at, ebay.ch, ebay.ie, ebay.nl, ebay.pl, ebay.be, ebay.com.sg, ebay.com.my, ebay.ph, ebay.com.hk | JSON-LD, CSS selectors | title, brand, MPN, GTIN, price, currency, availability, category, images, specifications, supplier, supplier URL, shipping |
| Etsy | `etsy` | Global | etsy.com | JSON-LD, CSS selectors | title, description, brand, SKU, price, currency, availability, category, images, specifications, supplier, supplier URL, canonical URL, rating, rating count |
| 1688.com | `alibaba-1688` | China | 1688.com | inline-script marker, CSS selectors | title, SKU, price, images, specifications, supplier, supplier URL, variants (property join) |
| JD.com | `jd` | China | jd.com, jd.hk | inline-script marker, CSS selectors | title, brand, SKU, model, price, category, images, specifications, supplier, supplier URL, variants (list) |
| Taobao | `taobao` | China | taobao.com | inline-script marker, CSS selectors | title, brand, SKU, price, availability, images, specifications, supplier, supplier URL, shipping, variants (property join) |
| Tmall | `tmall` | China | tmall.com, tmall.hk | inline-script marker, CSS selectors | title, brand, SKU, model, price, availability, images, specifications, supplier, supplier URL, variants (property join) |
| Alibaba.com | `alibaba-com` | Cross-border | alibaba.com | inline-script marker, CSS selectors | title, brand, model, price, images, specifications, supplier, supplier URL, shipping, variants (property join) |
| AliExpress | `aliexpress` | Cross-border | aliexpress.com, aliexpress.us | inline-script marker, CSS selectors | title, description, brand, model, price, list price, availability, category, images, specifications, supplier, supplier URL, shipping, rating, rating count, variants (property join) |
| Banggood | `banggood` | Cross-border | banggood.com | JSON-LD, CSS selectors | title, description, brand, SKU, price, currency, list price, availability, category, images, specifications, shipping, rating, rating count |
| CJdropshipping | `cjdropshipping` | Cross-border | cjdropshipping.com | script JSON, CSS selectors | title, description, SKU, price, category, images, specifications, variants (list) |
| DHgate | `dhgate` | Cross-border | dhgate.com | JSON-LD, inline-script marker, CSS selectors | title, SKU, price, currency, category, images, specifications, supplier, supplier URL, shipping, variants (property join) |
| Geekbuying | `geekbuying` | Cross-border | geekbuying.com | CSS selectors | title, SKU, price, list price, category, images, specifications, shipping |
| Rosegal | `rosegal` | Cross-border | rosegal.com | CSS selectors | title, SKU, price, list price, category, images, specifications |
| SaleYee | `saleyee` | Cross-border | saleyee.com | CSS selectors | title, description, SKU, price, list price, images, specifications, shipping |
| SHEIN | `shein` | Cross-border | shein.com, shein.co.uk | inline-script marker, CSS selectors | title, SKU, price, list price, category, images, specifications, rating, rating count, variants (list) |
| Temu | `temu` | Cross-border | temu.com | inline-script marker, CSS selectors | title, SKU, price, currency, list price, category, images, specifications, supplier, shipping, rating, rating count, variants (list) |
| Tomtop | `tomtop` | Cross-border | tomtop.com | JSON-LD, CSS selectors | title, description, brand, SKU, price, list price, category, images, specifications, shipping |
| Wish | `wish` | Cross-border | wish.com | inline-script marker, CSS selectors | title, description, price, list price, images, supplier, supplier URL, shipping, rating, rating count, variants (list) |
| AllStarHealth | `allstarhealth` | North America | allstarhealth.com | CSS selectors | title, description, brand, GTIN, price, list price, availability, images, specifications |
| Bed Bath & Beyond | `bedbathandbeyond` | North America | bedbathandbeyond.com | script JSON, CSS selectors | title, description, brand, SKU, price, currency, list price, category, images, specifications, rating, rating count, variants (list) |
| Bonanza | `bonanza` | North America | bonanza.com | CSS selectors | title, description, price, images, specifications, supplier, supplier URL, shipping |
| Costco | `costco` | North America | costco.com, costco.ca, costco.co.uk, costco.com.au, costco.co.jp, costco.co.kr, costco.com.mx, costco.com.tw | JSON-LD, CSS selectors | title, description, brand, SKU, price, category, images, specifications, shipping |
| Kohl's | `kohls` | North America | kohls.com | inline-script marker, CSS selectors | title, description, brand, SKU, price, list price, category, images, specifications, rating, rating count, variants (list) |
| LTD Commodities | `ltdcommodities` | North America | ltdcommodities.com | JSON-LD, CSS selectors | title, description, SKU, price, currency, list price, availability, images, specifications |
| Nordstrom | `nordstrom` | North America | nordstrom.com, nordstromrack.com | inline-script marker, CSS selectors | title, description, brand, SKU, model, price, list price, category, images, specifications, rating, rating count, variants (list) |
| Walmart | `walmart` | North America | walmart.com, walmart.ca | script JSON, CSS selectors | title, description, brand, SKU, model, GTIN, price, currency, list price, availability, category, images, specifications, supplier, shipping, rating, rating count, variants (property join) |
| Flipkart | `flipkart` | South Asia | flipkart.com | inline-script marker, CSS selectors | title, brand, SKU, model, price, list price, category, images, specifications, supplier, shipping, rating, rating count, variants (list) |
| Snapdeal | `snapdeal` | South Asia | snapdeal.com | JSON-LD, CSS selectors | title, brand, SKU, price, list price, images, specifications, supplier, supplier URL |
| Lazada | `lazada` | Southeast Asia | lazada.sg, lazada.com.my, lazada.com.ph, lazada.co.th, lazada.vn, lazada.co.id | inline-script marker, CSS selectors | title, description, brand, price, list price, availability, category, images, specifications, supplier, supplier URL, shipping, rating, rating count, variants (property join) |
| Shopee | `shopee` | Southeast Asia | shopee.sg, shopee.com.my, shopee.ph, shopee.co.th, shopee.vn, shopee.co.id, shopee.tw, shopee.com.br, shopee.com.mx, shopee.com.co, shopee.cl | JSON-LD, CSS selectors | title, description, brand, SKU, price, currency, availability, images, specifications, supplier, supplier URL, canonical URL, rating, rating count |

Domains include `www.`, `m.` and other subdomains (`*.amazon.de`, `de.aliexpress.com`,
`detail.tmall.com`, …). A definition also needs its **product signal** to hold before it reads
anything, so search results, category pages and seller pages on the same domains are not taken
for products.

## Official APIs: the better route for monitoring

Capturing a page is right for importing a product you are looking at. To monitor prices or stock
of many products, use the marketplace's own API where you have access to one; REA does not include
clients for them:

| Marketplace | Official API |
| --- | --- |
| CJdropshipping | CJ Dropshipping API (product, variant, inventory and order endpoints for CJ accounts) |
| AliExpress | AliExpress Open Platform, dropshipping API (product and SKU details for affiliates and dropshippers) |
| Etsy | Etsy Open API v3 (listings, inventory and shops) |
| Walmart | Walmart Marketplace APIs (sellers) and the Walmart affiliate product APIs |
| Amazon | Amazon Product Advertising API (affiliates) and the Selling Partner API (sellers) |
| eBay | eBay Browse API |

Their terms decide what you may do with the data; REA never calls them on its own.

## Managing definitions in the dashboard

**Products → Marketplaces** (permission `products.read`) lists the definitions grouped by region
with their domains, definition version, verification status, assumed page layout and the fields
each one extracts. With `settings.manage` you can:

- **Enable or disable** a definition. The extension stops using a disabled one the next time it
  loads its configuration.
- **Change its priority** (lower runs first). Priority only breaks ties: a value from a definition
  normally outranks the generic adapters by confidence.
- **Create, edit and delete custom rules adapters**: your own rule documents for sites REA does
  not ship, or replacements for a shipped definition (see below). The form validates the document
  as you type with the same schema the extension uses; the server validates it again.

Shipped definitions cannot be deleted or edited, only switched on or off and re-prioritised. At
start-up, after migrations, the API upserts the shipped definitions: missing ones are added
(enabled), and a definition whose shipped `version` is newer than the stored one replaces the
stored name, domains, rules, region and verification **while your enabled flag, priority and
description are kept**. A custom adapter you created with the same key as a shipped definition is
never overwritten (the API logs a warning).

## Adapter lab

**Products → Adapter lab** runs `@rea/extractor`, the same code as the extension, on a product page
you saved:

1. In your browser open a product page and save it (`Ctrl`+`S`, "Webpage, HTML only" is enough).
2. In the lab paste the HTML or choose the file, and enter the page address (filled in
   automatically when the browser recorded it in the saved file).
3. Choose what to run: all enabled adapters (what the extension would do on that address), one
   adapter or definition (also disabled ones, and also when the address is outside its domains), or
   a draft rule document. Optionally include the generic fallbacks.
4. **Read the page.** The lab shows every merged field with its value, confidence, adapter and
   evidence (the rule, source and path or selector it came from), what the chosen adapter found on
   its own, images, variants, specifications, offer metadata, warnings and every adapter that ran.

The saved HTML is parsed with `DOMParser` into an inert document. It is never inserted into the
dashboard, its scripts never run, and nothing it references is loaded: images are listed as links,
not shown. Nothing is sent to REA unless you save a draft as an adapter.

## When a marketplace changes its pages

Signs: the extension warns "no value was found for title/price/images; the page layout may have
changed", a field comes from the generic adapter instead of the definition, or values are wrong.

**Fix it for your installation right away:**

1. Save a current product page and open it in the Adapter lab with the definition selected. The
   warnings say which source could not be read and why (for example "the marker was not found").
2. Choose **Copy its rule document into a draft**, correct the marker, path or selector, and read
   the page again until the fields are right. Check a few different products.
3. **Save draft as a rules adapter** with a new key, the marketplace's domains and a lower priority
   number, then disable the shipped definition on the Marketplaces page.

**Fix it in REA itself** (for everyone):

1. Edit `db/marketplaces/<key>.json`: update `rules`, raise `version` by one and update the
   `verification` note to describe the new layout.
2. Update `testdata/fixtures/marketplaces/<key>.html` so it reproduces the new layout with invented
   data, and the expectations in `packages/extractor/tests/marketplaces.spec.ts`.
3. Run `pnpm --filter @rea/extractor test` and `go test ./services/internal/marketplaces`.
   Installations pick up the new version at their next start-up; their enabled flags and
   priorities are kept.
4. Only change `verification.status` when it is true: `saved_page` after checking real saved pages,
   `live_page` after checking with the extension on live pages. Say in the note what was checked
   and when.

## Rule documents

A rule document is JSON data (`"schema": 1`, at most 64 kB). It cannot run code, contain regular
expressions, make requests or read cookies, storage, form values or hidden inputs; the extractor and
the API both reject anything outside this format. Example (abridged from `aliexpress.json`):

```json
{
  "schema": 1,
  "product_signal": { "any": [{ "source": "run_params", "path": "data.productInfoComponent.subject" }] },
  "sources": { "run_params": { "kind": "script_marker", "marker": "window.runParams" } },
  "fields": {
    "title": [
      { "source": "run_params", "path": "data.productInfoComponent.subject", "transforms": ["trim"] },
      { "css": "h1[data-pl='product-title']" },
      { "meta": "og:title" }
    ],
    "price": [{ "source": "run_params", "path": "data.priceComponent.discountPrice.minActivityAmount" }],
    "brand": [{ "source": "run_params", "path": "data.productPropComponent.props[?attrName='Brand Name'].attrValue" }],
    "images": [{ "source": "run_params", "path": "data.imageComponent.imagePathList", "transforms": ["protocol_fix", "image_original"] }]
  },
  "variants": {
    "mode": "property_join",
    "properties": { "source": "run_params", "path": "data.skuComponent.productSKUPropertyList", "id": "skuPropertyId",
      "name": "skuPropertyName", "values": "skuPropertyValues", "value_id": "propertyValueId", "value_name": "propertyValueDisplayName" },
    "skus": { "source": "run_params", "path": "data.priceComponent.skuPriceList", "ids": "skuPropIds", "separator": ",",
      "sku": "skuId", "price": "skuVal.skuActivityAmount", "stock": "skuVal.availQuantity" }
  }
}
```

### Sources

Named JSON data sources, read lazily and at most once per page:

| `kind` | Reads |
| --- | --- |
| `json_ld` (`types`) | the first JSON-LD node with one of the schema.org types, e.g. `["Product", "ProductGroup"]` |
| `script_json` (`selector`) | the JSON text of a `<script>` element, e.g. `script#__NEXT_DATA__` |
| `script_marker` (`marker`, optional `selector`) | the value assigned after a literal marker in an inline script, e.g. `window.runParams`, `__INIT_DATA__`, `window.__INITIAL_STATE__`, `rawData`, `productIntroData` |
| `attr_json` (`selector`, `attr`) | JSON stored in an attribute, e.g. `#landingImage` `data-a-dynamic-image` |

A marker is plain text, never a regular expression. After the marker the extractor skips an
optional closing quote and one `=`, `:` or `(`, then reads an object or array literal (found with a
string-aware brace scanner and parsed as JSON, or as a JavaScript object literal with unquoted or
single-quoted keys, trailing commas, `undefined` and `!0`/`!1`), a string holding JSON, or
`JSON.parse("…")`. Functions, calls and other code make the source fail. Scripts over 2 MB are
skipped, at most 8 MB of inline script is examined per page, nesting is limited to 64 levels, and
keys that look like tokens, CSRF values, sessions or passwords are removed before anything reads
the data.

### Fields and candidates

`fields` maps each field to up to 10 candidates, tried in order; the first that yields a usable
value wins. Fields: `title`, `description`, `brand`, `sku`, `model`, `mpn`, `gtin`, `price`,
`currency`, `list_price`, `availability`, `category`, `images`, `specifications`, `supplier_name`,
`supplier_url`, `canonical_url`, `shipping`, `rating`, `rating_count` (`title` is required).

A candidate reads exactly one of:

- `source` + `path` (optionally `entry: {name, value}` to turn a list of objects into
  specifications);
- `meta`: a meta tag name or property (token-like names are refused);
- `css`: a selector, reading text (or `multiline` text), an `attr`ibute (never `value`, `on…` or
  token-like names), `all` matches (lists, images, breadcrumbs), a `table` (rows of a table or
  definition list) or `pairs: {name, value}` (name/value child selectors per match). Form controls
  are never read. Selectors cannot contain `<`, `>`, `{` or `}`, so use descendant selectors.

Optional `transforms` (applied in order) and `confidence` (0.1–0.99) complete a candidate.
Defaults: 0.97 for embedded data, 0.96 for CSS and 0.90 for meta tags, so a definition outranks
the generic JSON-LD adapter (0.95) and the generic adapters fill only what it leaves empty.

**Prices** accept numbers, price text or `{value|amount|price, currency|currencyCode|currencyUnit}`
objects. The currency comes from unambiguous price text, then the `currency` field, then
`currency_by_host` (longest matching host suffix, e.g. `"amazon.co.uk": "GBP"`), then the document's
`currency`, then an ambiguous symbol (with a warning).

**Offer metadata**: `list_price`, `shipping`, `rating` and `rating_count` are reported with
evidence, saved with the capture (`extracted.metadata`) and shown in the lab, but never imported as
product fields.

### Path language

| Path | Meaning |
| --- | --- |
| `a.b.c` | object keys (letters, digits, `_`, `$`, `-`) |
| `a[0]`, `a[-1]` | array index, negative from the end |
| `a[*].b`, `a.*.b` | every element of an array or value of an object; keys after a wildcard reach into arrays |
| `a['odd key']` | quoted key |
| `a[?name=Brand].value`, `a[?name='Brand Name']` | array elements whose `name` equals the text |
| `""` | the value itself |
| `$key` | inside variants, the key of the object entry being read |

Paths are at most 300 characters and 40 segments; wildcards return at most 500 matches. `__proto__`,
`prototype`, `constructor` and token-like key names are refused.

### Transforms

`trim`, `strip_html`, `number`, `cents` (integer cents to units), `first`, `last`, `join` (default
`" > "`) or `{"join": ", "}`, `{"split": "/"}`, `{"prefix": "https://img.example/"}` (for relative
image paths; must be an http(s) or protocol-relative URL), `absolute_url`, `protocol_fix`
(`//host/x` to `https://host/x`), `image_original` (thumbnail to original image: AliExpress,
Alibaba and Lazada `…jpg_220x220q75.jpg_.webp` and `…_640x640.png`, Amazon `._AC_SX679_.jpg`, eBay
`s-l300.jpg` to `s-l1600.jpg`, Shopee `_tn`, JD `/n5/` to `/n0/`, Flipkart `{@width}/{@height}`),
`lowercase`, `uppercase`, `digits`, `stock` (numbers and booleans to "in stock"/"out of stock") and
`keys` (an object's keys).

### Variants

- **`list`**: `source` + `path` to a list (or an object, whose entries expose `$key`), with item
  paths `title`, `sku`, `price`, `currency`, `stock`, `availability`, `image`, and options from
  `options` (`{"Color": "color"}`), `option_list` (`{path, name, value}` per item),
  `option_positions` (values by position, names from another source, as in Amazon's twister data)
  or `options_rest` (every other scalar key, minus `exclude`).
- **`property_join`**: for platforms that store option catalogues apart from SKU price lists
  (AliExpress `skuPropIds`, Alibaba.com, DHgate, 1688, Taobao, Tmall, Lazada, Walmart).
  `properties` describes the catalogue (`name`, `values`, `value_id`, `value_name`, optional `id`
  and `value_image`); `skus` lists the SKUs and where their value ids are (`ids`: a list or text
  split by `separator`, with `pid:vid` pairs supported, or `match: "name"` to join by value names);
  optional `offers` looks prices and stock up in a separate map by `key` (Taobao `sku2info`, Lazada
  `skuInfos`).

When no price candidate matches, the lowest variant price is used, with a warning when variants
differ.

### Product signal

`product_signal` has `any` and/or `all` lists of conditions: a `source` (+ `path`, optionally
`equals`) that must be present, a `css` selector that must match, a `meta` tag (optionally
`equals`) or a `url_path` substring. When the signal does not hold, the definition reads nothing
and reports that the page does not look like a product page.

## Where the code lives

| Path | Role |
| --- | --- |
| `db/marketplaces/<key>.json` | shipped definitions (key, name, region, description, domains, priority, version, verification, rules) |
| `testdata/fixtures/marketplaces/<key>.html` | synthetic fixtures, one per definition |
| `testdata/rules/validation-cases.json` | rule documents both validators must accept or reject |
| `packages/extractor/src/rules/` | `schema.ts` (zod), `json.ts` (scanner and parsers), `path.ts`, `transforms.ts`, `engine.ts` |
| `services/internal/marketplaces` | Go validation mirror, embedded definitions, start-up sync |
| `apps/web/src/views/products/{MarketplacesView,AdapterLabView}.vue` | dashboard pages |
