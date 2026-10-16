-- Product import manager: captured pages, source adapters, products, variants, images,
-- supplier links, import queue and opportunity line items.

-- +goose Up

CREATE TABLE source_adapters (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key             text NOT NULL UNIQUE CHECK (key ~ '^[a-z][a-z0-9_-]{1,40}$'),
    name            text NOT NULL,
    kind            text NOT NULL CHECK (kind IN ('builtin', 'selector')),
    domain_patterns text[] NOT NULL DEFAULT '{}',
    enabled         boolean NOT NULL DEFAULT true,
    priority        integer NOT NULL DEFAULT 100,
    -- CSS selector map for selector adapters, e.g. {"title": "h1.name", "price": ".price"}.
    selectors       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(selectors) = 'object'),
    description     text NOT NULL DEFAULT '',
    version         integer NOT NULL DEFAULT 1,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER source_adapters_touch BEFORE UPDATE ON source_adapters FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

INSERT INTO source_adapters (key, name, kind, domain_patterns, priority, description) VALUES
  ('json-ld', 'Schema.org JSON-LD', 'builtin', '{*}', 10, 'Reads schema.org Product, Offer and Organization data embedded as JSON-LD.'),
  ('microdata', 'Schema.org microdata', 'builtin', '{*}', 20, 'Reads itemprop/itemscope product markup.'),
  ('open-graph', 'Open Graph product tags', 'builtin', '{*}', 30, 'Reads og: and product: meta tags.'),
  ('shopify', 'Shopify storefronts', 'builtin', '{*.myshopify.com,*}', 40, 'Reads the product JSON that Shopify themes embed in the page.'),
  ('woocommerce', 'WooCommerce stores', 'builtin', '{*}', 40, 'Reads WooCommerce product markup and variation data.'),
  ('alibaba', 'Alibaba.com product pages', 'builtin', '{*.alibaba.com,alibaba.com}', 50, 'Reads visible Alibaba product details. Marketplace markup changes often; review every capture.'),
  ('generic', 'Generic page heuristics', 'builtin', '{*}', 1000, 'Fallback: headings, price patterns and prominent images. Lowest confidence.');

CREATE TABLE captured_pages (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    url               text NOT NULL CHECK (length(url) <= 2048),
    canonical_url     text CHECK (length(canonical_url) <= 2048),
    domain            text NOT NULL,
    title             text NOT NULL DEFAULT '',
    description       text NOT NULL DEFAULT '',
    page_type         text NOT NULL DEFAULT 'unknown' CHECK (page_type IN
                      ('product', 'listing', 'company', 'contact', 'article', 'documentation', 'unknown')),
    adapter_key       text NOT NULL DEFAULT 'generic',
    extraction_method text NOT NULL DEFAULT 'generic',
    -- Raw structured metadata found on the page (JSON-LD blocks, Open Graph, meta tags).
    structured_data   jsonb NOT NULL DEFAULT '{}',
    -- The extractor's output (product payload, page outline).
    extracted         jsonb NOT NULL DEFAULT '{}',
    -- User-selected text, tables, elements and images.
    selections        jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(selections) = 'array'),
    warnings          text[] NOT NULL DEFAULT '{}',
    html_snapshot     text CHECK (length(html_snapshot) <= 3000000),
    content_sha256    text NOT NULL DEFAULT '',
    captured_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    device_id         uuid REFERENCES extension_devices(id) ON DELETE SET NULL,
    captured_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX captured_pages_domain_idx ON captured_pages (domain, captured_at DESC);
CREATE INDEX captured_pages_time_idx ON captured_pages (captured_at DESC);
CREATE INDEX captured_pages_user_idx ON captured_pages (captured_by, captured_at DESC);
CREATE INDEX captured_pages_device_idx ON captured_pages (device_id, captured_at DESC);
CREATE INDEX captured_pages_search_idx ON captured_pages USING gin (rea_search_doc(title, description, domain, url));
CREATE TRIGGER captured_pages_cleanup AFTER DELETE ON captured_pages FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('captured_page');

CREATE TABLE products (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    original_title        text NOT NULL CHECK (length(original_title) BETWEEN 1 AND 500),
    title                 text NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    normalized_title      text NOT NULL,
    source_url            text CHECK (length(source_url) <= 2048),
    canonical_url         text CHECK (length(canonical_url) <= 2048),
    source_domain         text NOT NULL DEFAULT '',
    supplier_id           uuid REFERENCES suppliers(id) ON DELETE SET NULL,
    supplier_contact_id   uuid REFERENCES contacts(id) ON DELETE SET NULL,
    company_id            uuid REFERENCES companies(id) ON DELETE SET NULL,
    brand                 text NOT NULL DEFAULT '',
    sku                   text NOT NULL DEFAULT '',
    model                 text NOT NULL DEFAULT '',
    mpn                   text NOT NULL DEFAULT '',
    gtin                  text CHECK (gtin ~ '^[0-9]{8,14}$'),
    -- Additional identifiers whose names vary by marketplace (ASIN, item number, ...).
    identifiers           jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(identifiers) = 'object'),
    description           text NOT NULL DEFAULT '',
    -- Free-form specification table (name -> value); genuinely variable per product.
    specifications        jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(specifications) = 'object'),
    cost                  numeric(14, 4) CHECK (cost >= 0),
    cost_currency         char(3) CHECK (cost_currency ~ '^[A-Z]{3}$'),
    price                 numeric(14, 4) CHECK (price >= 0),
    price_currency        char(3) CHECK (price_currency ~ '^[A-Z]{3}$'),
    margin_amount         numeric(14, 4) GENERATED ALWAYS AS (
                              CASE WHEN cost IS NOT NULL AND price IS NOT NULL AND cost_currency = price_currency
                                   THEN price - cost END) STORED,
    margin_percent        numeric(7, 2) GENERATED ALWAYS AS (
                              CASE WHEN cost IS NOT NULL AND price IS NOT NULL AND price > 0 AND cost_currency = price_currency
                                   THEN round((price - cost) / price * 100, 2) END) STORED,
    availability          text NOT NULL DEFAULT 'unknown' CHECK (availability IN
                          ('in_stock', 'out_of_stock', 'preorder', 'backorder', 'discontinued', 'unknown')),
    category              text NOT NULL DEFAULT '',
    status                text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'archived')),
    review_status         text NOT NULL DEFAULT 'pending' CHECK (review_status IN ('pending', 'needs_changes', 'approved', 'rejected')),
    first_seen_at         timestamptz NOT NULL DEFAULT now(),
    last_checked_at       timestamptz NOT NULL DEFAULT now(),
    extraction_method     text NOT NULL DEFAULT 'manual',
    extraction_confidence numeric(4, 3) CHECK (extraction_confidence BETWEEN 0 AND 1),
    captured_page_id      uuid REFERENCES captured_pages(id) ON DELETE SET NULL,
    -- Selected evidence the values were extracted from (field -> excerpt/selector).
    source_evidence       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(source_evidence) = 'object'),
    internal_notes        text NOT NULL DEFAULT '',
    custom_fields         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by            uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_search_idx ON products USING gin (rea_search_doc(title, brand, sku, description));
CREATE INDEX products_title_trgm_idx ON products USING gin (normalized_title gin_trgm_ops);
CREATE INDEX products_canonical_idx ON products (canonical_url) WHERE canonical_url IS NOT NULL;
CREATE INDEX products_gtin_idx ON products (gtin) WHERE gtin IS NOT NULL;
CREATE INDEX products_domain_sku_idx ON products (source_domain, lower(sku)) WHERE sku <> '';
CREATE INDEX products_brand_mpn_idx ON products (lower(brand), lower(mpn)) WHERE mpn <> '';
CREATE INDEX products_supplier_idx ON products (supplier_id);
CREATE INDEX products_status_idx ON products (status, review_status, updated_at DESC);
CREATE INDEX products_updated_idx ON products (updated_at DESC);
CREATE TRIGGER products_touch BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();
CREATE TRIGGER products_cleanup AFTER DELETE ON products FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('product');

CREATE TABLE product_variants (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id   uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    title        text NOT NULL DEFAULT '',
    sku          text NOT NULL DEFAULT '',
    -- Option name -> value, e.g. {"Color": "Red", "Size": "M"}.
    options      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(options) = 'object'),
    price        numeric(14, 4) CHECK (price >= 0),
    currency     char(3) CHECK (currency ~ '^[A-Z]{3}$'),
    availability text NOT NULL DEFAULT 'unknown' CHECK (availability IN
                 ('in_stock', 'out_of_stock', 'preorder', 'backorder', 'discontinued', 'unknown')),
    image_url    text CHECK (length(image_url) <= 2048),
    position     integer NOT NULL DEFAULT 0,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX product_variants_product_idx ON product_variants (product_id, position);
CREATE TRIGGER product_variants_touch BEFORE UPDATE ON product_variants FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE product_images (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id    uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    url           text NOT NULL CHECK (length(url) <= 2048),
    source_page   text CHECK (length(source_page) <= 2048),
    alt_text      text NOT NULL DEFAULT '',
    width         integer CHECK (width > 0),
    height        integer CHECK (height > 0),
    position      integer NOT NULL DEFAULT 0,
    is_primary    boolean NOT NULL DEFAULT false,
    attachment_id uuid REFERENCES attachments(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, url)
);
CREATE INDEX product_images_product_idx ON product_images (product_id, position);

CREATE TABLE supplier_products (
    supplier_id    uuid NOT NULL REFERENCES suppliers(id) ON DELETE CASCADE,
    product_id     uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    supplier_sku   text NOT NULL DEFAULT '',
    cost           numeric(14, 4) CHECK (cost >= 0),
    currency       char(3) CHECK (currency ~ '^[A-Z]{3}$'),
    moq            integer CHECK (moq >= 0),
    lead_time_days integer CHECK (lead_time_days >= 0),
    is_preferred   boolean NOT NULL DEFAULT false,
    notes          text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (supplier_id, product_id)
);
CREATE INDEX supplier_products_product_idx ON supplier_products (product_id);
CREATE TRIGGER supplier_products_touch BEFORE UPDATE ON supplier_products FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE product_imports (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    source                 text NOT NULL CHECK (source IN ('extension', 'csv', 'manual', 'analysis', 'api')),
    status                 text NOT NULL DEFAULT 'pending' CHECK (status IN
                           ('pending', 'imported', 'updated', 'linked', 'duplicate', 'rejected', 'failed')),
    decision               text CHECK (decision IN ('create_new', 'update_existing', 'skip')),
    -- Raw payload as submitted and the normalized form produced by the server.
    payload                jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    normalized             jsonb,
    warnings               text[] NOT NULL DEFAULT '{}',
    error                  text NOT NULL DEFAULT '',
    duplicate_candidates   jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(duplicate_candidates) = 'array'),
    product_id             uuid REFERENCES products(id) ON DELETE SET NULL,
    duplicate_of_id        uuid REFERENCES products(id) ON DELETE SET NULL,
    captured_page_id       uuid REFERENCES captured_pages(id) ON DELETE SET NULL,
    device_id              uuid REFERENCES extension_devices(id) ON DELETE SET NULL,
    created_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    reviewed_by            uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at             timestamptz NOT NULL DEFAULT now(),
    processed_at           timestamptz
);
CREATE INDEX product_imports_status_idx ON product_imports (status, created_at DESC);
CREATE INDEX product_imports_product_idx ON product_imports (product_id);
CREATE INDEX product_imports_user_idx ON product_imports (created_by, created_at DESC);

CREATE TABLE opportunity_products (
    opportunity_id uuid NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
    product_id     uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity       numeric(14, 3) NOT NULL DEFAULT 1 CHECK (quantity > 0),
    unit_price     numeric(14, 4) CHECK (unit_price >= 0),
    currency       char(3) CHECK (currency ~ '^[A-Z]{3}$'),
    notes          text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (opportunity_id, product_id)
);
CREATE INDEX opportunity_products_product_idx ON opportunity_products (product_id);

-- +goose Down
DROP TABLE opportunity_products;
DROP TABLE product_imports;
DROP TABLE supplier_products;
DROP TABLE product_images;
DROP TABLE product_variants;
DROP TABLE products;
DROP TABLE captured_pages;
DROP TABLE source_adapters;
