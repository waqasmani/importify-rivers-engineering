-- Orders: customer orders received from the user's own systems (integration keys) or entered by
-- hand, and the purchase orders to suppliers that REA prepares from them. Purchase orders are
-- prepared automatically as drafts and are only handed off for placement after a signed-in person
-- confirms exactly what will be ordered (content_hash). REA never contacts a marketplace itself.

-- +goose Up

-- How a supplier's confirmed purchase orders are placed: by a person on the supplier's site
-- (manual) or by the user's own automation, which receives supplier_order.confirmed webhooks and
-- reports back with an integration key (automation). Nothing is ever placed without confirmation.
ALTER TABLE suppliers ADD COLUMN order_placement text NOT NULL DEFAULT 'manual'
    CHECK (order_placement IN ('manual', 'automation'));

CREATE SEQUENCE order_number_seq;
CREATE SEQUENCE supplier_order_number_seq;

-- Customer orders. Customer name, e-mail, phone, address and note are personal data: they are
-- never written to logs or the audit log, never sent to AI providers, and can be erased.
CREATE TABLE orders (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number             text NOT NULL UNIQUE DEFAULT ('SO-' || lpad(nextval('order_number_seq')::text, 6, '0')),
    channel            text NOT NULL CHECK (channel IN ('api', 'manual')),
    integration_key_id uuid REFERENCES integration_keys(id) ON DELETE SET NULL,
    created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    -- Who submitted it: the integration key's name or the user's name at intake.
    source_label       text NOT NULL DEFAULT '' CHECK (length(source_label) <= 200),
    -- The source system's order ID; unique per integration key (and among manual orders) so that
    -- repeated submissions are idempotent.
    external_id        text NOT NULL DEFAULT '' CHECK (length(external_id) <= 200),
    -- SHA-256 of the normalized submission: a repeat with the same contents returns the order, a
    -- repeat with different contents is refused.
    request_hash       text NOT NULL,
    customer_name      text NOT NULL DEFAULT '' CHECK (length(customer_name) <= 200),
    customer_email     text NOT NULL DEFAULT '' CHECK (length(customer_email) <= 320),
    customer_phone     text NOT NULL DEFAULT '' CHECK (length(customer_phone) <= 60),
    ship_name          text NOT NULL DEFAULT '' CHECK (length(ship_name) <= 200),
    ship_company       text NOT NULL DEFAULT '' CHECK (length(ship_company) <= 200),
    ship_line1         text NOT NULL DEFAULT '' CHECK (length(ship_line1) <= 300),
    ship_line2         text NOT NULL DEFAULT '' CHECK (length(ship_line2) <= 300),
    ship_city          text NOT NULL DEFAULT '' CHECK (length(ship_city) <= 120),
    ship_region        text NOT NULL DEFAULT '' CHECK (length(ship_region) <= 120),
    ship_postal_code   text NOT NULL DEFAULT '' CHECK (length(ship_postal_code) <= 40),
    ship_country       text NOT NULL DEFAULT '' CHECK (ship_country = '' OR ship_country ~ '^[A-Z]{2}$'),
    ship_phone         text NOT NULL DEFAULT '' CHECK (length(ship_phone) <= 60),
    -- What the customer paid, in currency (informational; REA never charges anyone).
    currency           char(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    subtotal           numeric(14, 4) CHECK (subtotal >= 0),
    shipping_amount    numeric(14, 4) CHECK (shipping_amount >= 0),
    tax_amount         numeric(14, 4) CHECK (tax_amount >= 0),
    total              numeric(14, 4) CHECK (total >= 0),
    customer_note      text NOT NULL DEFAULT '' CHECK (length(customer_note) <= 2000),
    ordered_at         timestamptz NOT NULL DEFAULT now(),
    -- Derived from the lines and supplier orders after every change.
    status             text NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'needs_attention',
                       'awaiting_confirmation', 'in_progress', 'placed', 'partially_shipped', 'shipped', 'delivered', 'cancelled')),
    needs_attention    boolean NOT NULL DEFAULT false,
    cancelled_at       timestamptz,
    cancelled_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    cancelled_by_label text NOT NULL DEFAULT '',
    cancel_reason      text NOT NULL DEFAULT '' CHECK (length(cancel_reason) <= 1000),
    customer_erased_at timestamptz,
    customer_erased_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT orders_api_external_id CHECK (channel <> 'api' OR external_id <> '')
);
CREATE UNIQUE INDEX orders_key_external_idx ON orders (integration_key_id, external_id) WHERE channel = 'api';
CREATE UNIQUE INDEX orders_manual_external_idx ON orders (external_id) WHERE channel = 'manual' AND external_id <> '';
CREATE INDEX orders_external_idx ON orders (external_id) WHERE external_id <> '';
CREATE INDEX orders_ordered_idx ON orders (ordered_at DESC, id);
CREATE INDEX orders_status_idx ON orders (status, ordered_at DESC);
CREATE INDEX orders_attention_idx ON orders (ordered_at DESC) WHERE needs_attention;
CREATE INDEX orders_key_idx ON orders (integration_key_id, ordered_at DESC) WHERE integration_key_id IS NOT NULL;
CREATE TRIGGER orders_touch BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- Purchase orders to one supplier (or, for products without a supplier record, one source domain).
-- Drafts are rebuilt from the order lines; from confirmation on, items, ship-to, supplier and
-- placement method are frozen and content_hash records what the person confirmed.
CREATE TABLE supplier_orders (
    id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    number                   text NOT NULL UNIQUE DEFAULT ('PO-' || lpad(nextval('supplier_order_number_seq')::text, 6, '0')),
    order_id                 uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    supplier_id              uuid REFERENCES suppliers(id) ON DELETE SET NULL,
    supplier_name            text NOT NULL DEFAULT '',
    source_domain            text NOT NULL DEFAULT '',
    -- "supplier:<id>" or "domain:<host>": lines of one order with the same key share a draft.
    group_key                text NOT NULL,
    placement_method         text NOT NULL DEFAULT 'manual' CHECK (placement_method IN ('manual', 'automation')),
    status                   text NOT NULL DEFAULT 'draft' CHECK (status IN
                             ('draft', 'confirmed', 'placed', 'failed', 'shipped', 'delivered', 'cancelled')),
    -- The items to buy (product, variant, options, quantity, source URL, expected cost).
    items                    jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(items) = 'array'),
    expected_cost_total      numeric(14, 4) CHECK (expected_cost_total >= 0),
    cost_currency            char(3) CHECK (cost_currency ~ '^[A-Z]{3}$'),
    ship_name                text NOT NULL DEFAULT '' CHECK (length(ship_name) <= 200),
    ship_company             text NOT NULL DEFAULT '' CHECK (length(ship_company) <= 200),
    ship_line1               text NOT NULL DEFAULT '' CHECK (length(ship_line1) <= 300),
    ship_line2               text NOT NULL DEFAULT '' CHECK (length(ship_line2) <= 300),
    ship_city                text NOT NULL DEFAULT '' CHECK (length(ship_city) <= 120),
    ship_region              text NOT NULL DEFAULT '' CHECK (length(ship_region) <= 120),
    ship_postal_code         text NOT NULL DEFAULT '' CHECK (length(ship_postal_code) <= 40),
    ship_country             text NOT NULL DEFAULT '' CHECK (ship_country = '' OR ship_country ~ '^[A-Z]{2}$'),
    ship_phone               text NOT NULL DEFAULT '' CHECK (length(ship_phone) <= 60),
    note_to_supplier         text NOT NULL DEFAULT '' CHECK (length(note_to_supplier) <= 2000),
    -- Confirmation: only a signed-in person with orders.place, in the dashboard.
    content_hash             text NOT NULL DEFAULT '',
    confirmed_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    confirmed_by_label       text NOT NULL DEFAULT '',
    confirmed_at             timestamptz,
    -- Hand-offs to automation (1 after confirmation, +1 per retry).
    attempt                  integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
    -- Placement, recorded by a person or reported by the user's automation.
    supplier_order_number    text NOT NULL DEFAULT '' CHECK (length(supplier_order_number) <= 200),
    actual_cost              numeric(14, 4) CHECK (actual_cost >= 0),
    actual_currency          char(3) CHECK (actual_currency ~ '^[A-Z]{3}$'),
    placement_note           text NOT NULL DEFAULT '' CHECK (length(placement_note) <= 1000),
    placed_by                uuid REFERENCES users(id) ON DELETE SET NULL,
    placed_by_key_id         uuid REFERENCES integration_keys(id) ON DELETE SET NULL,
    placed_by_label          text NOT NULL DEFAULT '',
    placed_at                timestamptz,
    failure_reason           text NOT NULL DEFAULT '' CHECK (length(failure_reason) <= 1000),
    failed_at                timestamptz,
    carrier                  text NOT NULL DEFAULT '' CHECK (length(carrier) <= 100),
    tracking_number          text NOT NULL DEFAULT '' CHECK (length(tracking_number) <= 200),
    tracking_url             text CHECK (length(tracking_url) <= 2048),
    shipped_at               timestamptz,
    delivered_at             timestamptz,
    cancelled_by             uuid REFERENCES users(id) ON DELETE SET NULL,
    cancelled_by_label       text NOT NULL DEFAULT '',
    cancelled_at             timestamptz,
    cancel_reason            text NOT NULL DEFAULT '' CHECK (length(cancel_reason) <= 1000),
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT supplier_orders_confirmed_has_hash CHECK (status IN ('draft', 'cancelled') OR (content_hash <> '' AND confirmed_at IS NOT NULL))
);
CREATE INDEX supplier_orders_order_idx ON supplier_orders (order_id, created_at);
CREATE INDEX supplier_orders_status_idx ON supplier_orders (status, updated_at DESC);
CREATE INDEX supplier_orders_supplier_idx ON supplier_orders (supplier_id) WHERE supplier_id IS NOT NULL;
CREATE TRIGGER supplier_orders_touch BEFORE UPDATE ON supplier_orders FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- Lines as sold, with what REA resolved them to and a snapshot of the sourcing data taken when
-- the purchase orders were prepared.
CREATE TABLE order_lines (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id             uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    position             integer NOT NULL CHECK (position >= 0),
    external_line_id     text NOT NULL DEFAULT '' CHECK (length(external_line_id) <= 200),
    sku                  text NOT NULL DEFAULT '' CHECK (length(sku) <= 200),
    title                text NOT NULL DEFAULT '' CHECK (length(title) <= 500),
    quantity             integer NOT NULL CHECK (quantity BETWEEN 1 AND 10000),
    unit_price           numeric(14, 4) CHECK (unit_price >= 0),
    -- Catalog IDs as submitted (they may not exist).
    requested_product_id uuid,
    requested_variant_id uuid,
    product_id           uuid REFERENCES products(id) ON DELETE SET NULL,
    variant_id           uuid REFERENCES product_variants(id) ON DELETE SET NULL,
    resolution           text NOT NULL DEFAULT 'unmatched' CHECK (resolution IN ('matched_variant', 'matched_product', 'unmatched', 'manual')),
    resolution_note      text NOT NULL DEFAULT '' CHECK (length(resolution_note) <= 300),
    supplier_id          uuid REFERENCES suppliers(id) ON DELETE SET NULL,
    supplier_name        text NOT NULL DEFAULT '',
    source_domain        text NOT NULL DEFAULT '',
    source_url           text CHECK (length(source_url) <= 2048),
    product_title        text NOT NULL DEFAULT '',
    variant_title        text NOT NULL DEFAULT '',
    variant_options      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(variant_options) = 'object'),
    expected_unit_cost   numeric(14, 4) CHECK (expected_unit_cost >= 0),
    cost_currency        char(3) CHECK (cost_currency ~ '^[A-Z]{3}$'),
    prepared_at          timestamptz,
    supplier_order_id    uuid REFERENCES supplier_orders(id) ON DELETE SET NULL,
    cancelled_at         timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (order_id, position)
);
CREATE INDEX order_lines_supplier_order_idx ON order_lines (supplier_order_id) WHERE supplier_order_id IS NOT NULL;
CREATE INDEX order_lines_product_idx ON order_lines (product_id) WHERE product_id IS NOT NULL;
CREATE TRIGGER order_lines_touch BEFORE UPDATE ON order_lines FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- The order's activity timeline (who did what, from/to status). Never contains personal data.
CREATE TABLE order_events (
    id                       bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id                 uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    supplier_order_id        uuid REFERENCES supplier_orders(id) ON DELETE SET NULL,
    kind                     text NOT NULL CHECK (length(kind) BETWEEN 1 AND 60),
    summary                  text NOT NULL CHECK (length(summary) <= 500),
    from_status              text NOT NULL DEFAULT '',
    to_status                text NOT NULL DEFAULT '',
    actor_user_id            uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_integration_key_id uuid REFERENCES integration_keys(id) ON DELETE SET NULL,
    actor_label              text NOT NULL DEFAULT '',
    created_at               timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_events_order_idx ON order_events (order_id, id);

-- Order webhook deliveries are created in the transaction that changes the order; these columns
-- find them again (for erasure of the customer data they carry).
ALTER TABLE webhook_deliveries
    ADD COLUMN order_id uuid,
    ADD COLUMN supplier_order_id uuid;
CREATE INDEX webhook_deliveries_order_idx ON webhook_deliveries (order_id) WHERE order_id IS NOT NULL;
CREATE INDEX webhook_deliveries_supplier_order_idx ON webhook_deliveries (supplier_order_id, event_type) WHERE supplier_order_id IS NOT NULL;

INSERT INTO permissions (code, description) VALUES
    ('orders.read',  'View customer orders, including customer names and shipping addresses, and supplier orders'),
    ('orders.write', 'Enter customer orders, map lines, edit drafts, record placements and shipments, cancel and erase customer data'),
    ('orders.place', 'Confirm supplier orders for placement (spends money) and withdraw or retry confirmations');

-- The administrator role received every permission at foundation time only, so grant these
-- explicitly. Viewers get none: orders carry customer personal data (administrators can grant it).
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN permissions p ON p.code IN ('orders.read', 'orders.write', 'orders.place')
WHERE r.key = 'admin';
INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN permissions p ON p.code IN ('orders.read', 'orders.write')
WHERE r.key IN ('analyst', 'member');

-- +goose Down
DELETE FROM role_permissions WHERE permission_code IN ('orders.read', 'orders.write', 'orders.place');
DELETE FROM permissions WHERE code IN ('orders.read', 'orders.write', 'orders.place');
DROP INDEX webhook_deliveries_supplier_order_idx;
DROP INDEX webhook_deliveries_order_idx;
ALTER TABLE webhook_deliveries DROP COLUMN supplier_order_id, DROP COLUMN order_id;
DROP TABLE order_events;
DROP TABLE order_lines;
DROP TABLE supplier_orders;
DROP TABLE orders;
DROP SEQUENCE supplier_order_number_seq;
DROP SEQUENCE order_number_seq;
ALTER TABLE suppliers DROP COLUMN order_placement;
