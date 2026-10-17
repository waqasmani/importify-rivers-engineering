-- Dropshipping pricing: the marketplace price observed at import becomes the product's source
-- price, cost defaults to it (converted into the base currency with manually maintained rates)
-- and the selling price comes from ordered pricing rules unless a user locked a manual price.

-- +goose Up

CREATE TABLE pricing_rules (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description                text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    enabled                    boolean NOT NULL DEFAULT true,
    -- Lower runs first; the default rule is always evaluated last whatever its priority.
    priority                   integer NOT NULL DEFAULT 100 CHECK (priority BETWEEN 0 AND 1000000),
    is_default                 boolean NOT NULL DEFAULT false,
    -- Conditions. Every condition that is set must hold; list conditions match any entry.
    source_domains             text[] NOT NULL DEFAULT '{}',
    supplier_ids               uuid[] NOT NULL DEFAULT '{}',
    category_contains          text NOT NULL DEFAULT '' CHECK (length(category_contains) <= 300),
    tag_ids                    uuid[] NOT NULL DEFAULT '{}',
    -- Cost range in the base currency: min_cost <= cost < max_cost.
    min_cost                   numeric(14, 4) CHECK (min_cost >= 0),
    max_cost                   numeric(14, 4) CHECK (max_cost > 0),
    -- Actions, applied in this order: fixed amount (when before markup), multiplier, markup,
    -- fixed amount (when after markup), minimums, rounding, compare-at price.
    markup_percent             numeric(9, 4) CHECK (markup_percent BETWEEN 0 AND 10000),
    multiplier                 numeric(9, 4) CHECK (multiplier > 0 AND multiplier <= 1000),
    fixed_amount               numeric(14, 4) CHECK (fixed_amount >= 0),
    fixed_amount_before_markup boolean NOT NULL DEFAULT false,
    min_price                  numeric(14, 4) CHECK (min_price >= 0),
    min_margin_percent         numeric(5, 2) CHECK (min_margin_percent >= 0 AND min_margin_percent < 100),
    min_margin_amount          numeric(14, 4) CHECK (min_margin_amount >= 0),
    rounding                   text NOT NULL DEFAULT 'none' CHECK (rounding IN ('none', 'ending', 'step')),
    rounding_ending            numeric(3, 2) CHECK (rounding_ending >= 0 AND rounding_ending < 1),
    rounding_step              numeric(14, 4) CHECK (rounding_step > 0),
    compare_at_mode            text NOT NULL DEFAULT 'none' CHECK (compare_at_mode IN ('none', 'multiplier', 'percent')),
    compare_at_value           numeric(9, 4) CHECK (compare_at_value > 0),
    created_by                 uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT pricing_rules_cost_range CHECK (min_cost IS NULL OR max_cost IS NULL OR max_cost > min_cost),
    CONSTRAINT pricing_rules_ending_set CHECK (rounding <> 'ending' OR rounding_ending IS NOT NULL),
    CONSTRAINT pricing_rules_step_set CHECK (rounding <> 'step' OR rounding_step IS NOT NULL),
    CONSTRAINT pricing_rules_compare_at_set CHECK (compare_at_mode = 'none' OR compare_at_value IS NOT NULL),
    CONSTRAINT pricing_rules_default_unconditional CHECK (NOT is_default OR (source_domains = '{}' AND supplier_ids = '{}'
        AND category_contains = '' AND tag_ids = '{}' AND min_cost IS NULL AND max_cost IS NULL))
);
CREATE UNIQUE INDEX pricing_rules_single_default ON pricing_rules (is_default) WHERE is_default;
CREATE INDEX pricing_rules_order_idx ON pricing_rules (is_default, priority, created_at);
CREATE TRIGGER pricing_rules_touch BEFORE UPDATE ON pricing_rules FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- The fallback rule keeps today's behaviour (selling price = cost) until an administrator adds a markup.
INSERT INTO pricing_rules (name, description, is_default, priority) VALUES
  ('Default rule', 'Used when no other rule matches. It starts with selling price = cost; add your markup here.', true, 1000000);

-- Manually maintained exchange rates: one unit of currency is worth rate units of base_currency.
-- REA never fetches rates from the internet.
CREATE TABLE currency_rates (
    base_currency char(3) NOT NULL CHECK (base_currency ~ '^[A-Z]{3}$'),
    currency      char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    rate          numeric(20, 10) NOT NULL CHECK (rate > 0),
    note          text NOT NULL DEFAULT '' CHECK (length(note) <= 300),
    updated_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (base_currency, currency),
    CONSTRAINT currency_rates_not_base CHECK (base_currency <> currency)
);
CREATE TRIGGER currency_rates_touch BEFORE UPDATE ON currency_rates FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

ALTER TABLE products
    ADD COLUMN source_price     numeric(14, 4) CHECK (source_price >= 0),
    ADD COLUMN source_currency  char(3) CHECK (source_currency ~ '^[A-Z]{3}$'),
    ADD COLUMN cost_manual      boolean NOT NULL DEFAULT false,
    ADD COLUMN compare_at_price numeric(14, 4) CHECK (compare_at_price >= 0),
    ADD COLUMN price_locked     boolean NOT NULL DEFAULT false,
    ADD COLUMN pricing_rule_id  uuid REFERENCES pricing_rules(id) ON DELETE SET NULL,
    ADD COLUMN priced_at        timestamptz;
CREATE INDEX products_pricing_rule_idx ON products (pricing_rule_id) WHERE pricing_rule_id IS NOT NULL;

ALTER TABLE product_variants
    ADD COLUMN source_price     numeric(14, 4) CHECK (source_price >= 0),
    ADD COLUMN source_currency  char(3) CHECK (source_currency ~ '^[A-Z]{3}$'),
    ADD COLUMN cost             numeric(14, 4) CHECK (cost >= 0),
    ADD COLUMN cost_currency    char(3) CHECK (cost_currency ~ '^[A-Z]{3}$'),
    ADD COLUMN cost_manual      boolean NOT NULL DEFAULT false,
    ADD COLUMN compare_at_price numeric(14, 4) CHECK (compare_at_price >= 0),
    ADD COLUMN price_locked     boolean NOT NULL DEFAULT false,
    ADD COLUMN pricing_rule_id  uuid REFERENCES pricing_rules(id) ON DELETE SET NULL,
    ADD COLUMN priced_at        timestamptz;
CREATE INDEX product_variants_pricing_rule_idx ON product_variants (pricing_rule_id) WHERE pricing_rule_id IS NOT NULL;

-- Backfill. Prices stored so far were observed marketplace prices: copy them to the source price
-- and leave the selling price as it is, unlocked. Imports never set a cost, so any existing cost
-- was entered by a person and is kept as a manual cost. The backfill is not a user edit, so the
-- updated_at triggers are paused while it runs.
ALTER TABLE products DISABLE TRIGGER products_touch;
ALTER TABLE product_variants DISABLE TRIGGER product_variants_touch;
UPDATE products SET source_price = price, source_currency = price_currency WHERE price IS NOT NULL;
UPDATE products SET cost_manual = true WHERE cost IS NOT NULL;
UPDATE product_variants SET source_price = price, source_currency = currency WHERE price IS NOT NULL;
ALTER TABLE products ENABLE TRIGGER products_touch;
ALTER TABLE product_variants ENABLE TRIGGER product_variants_touch;

-- +goose Down
ALTER TABLE product_variants
    DROP COLUMN priced_at,
    DROP COLUMN pricing_rule_id,
    DROP COLUMN price_locked,
    DROP COLUMN compare_at_price,
    DROP COLUMN cost_manual,
    DROP COLUMN cost_currency,
    DROP COLUMN cost,
    DROP COLUMN source_currency,
    DROP COLUMN source_price;
ALTER TABLE products
    DROP COLUMN priced_at,
    DROP COLUMN pricing_rule_id,
    DROP COLUMN price_locked,
    DROP COLUMN compare_at_price,
    DROP COLUMN cost_manual,
    DROP COLUMN source_currency,
    DROP COLUMN source_price;
DROP TABLE currency_rates;
DROP TABLE pricing_rules;
