-- Catalog API: integration keys for machine clients, change capture for the published catalog
-- (an outbox written by triggers on every write path), and outgoing webhooks.

-- +goose Up

-- Integration keys belong to the organization, not to a person. Their scopes are fixed when the
-- key is created and are never intersected with a user's role, so deleting or downgrading the
-- administrator who created a key neither breaks nor widens it. created_by is kept for the audit
-- trail only.
CREATE TABLE integration_keys (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description      text NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    token_hash       bytea NOT NULL UNIQUE,
    token_prefix     text NOT NULL,
    scopes           text[] NOT NULL CHECK (cardinality(scopes) BETWEEN 1 AND 20),
    expires_at       timestamptz,
    last_used_at     timestamptz,
    last_used_ip     text NOT NULL DEFAULT '',
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by_label text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    revoked_at       timestamptz,
    revoked_by       uuid REFERENCES users(id) ON DELETE SET NULL
);
CREATE INDEX integration_keys_created_idx ON integration_keys (created_at DESC);

-- Request counts per key and day (the audit log records the first use per hour and IP).
CREATE TABLE integration_key_usage (
    key_id   uuid NOT NULL REFERENCES integration_keys(id) ON DELETE CASCADE,
    day      date NOT NULL,
    requests bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (key_id, day)
);

ALTER TABLE audit_logs ADD COLUMN actor_integration_key_id uuid REFERENCES integration_keys(id) ON DELETE SET NULL;
CREATE INDEX audit_logs_integration_key_idx ON audit_logs (actor_integration_key_id, occurred_at DESC)
    WHERE actor_integration_key_id IS NOT NULL;

-- A product is in the published catalog when it is active and its review is approved.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_published(p_status text, p_review_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT p_status = 'active' AND p_review_status = 'approved'
$$;
-- +goose StatementEnd

-- Product columns that are not part of the catalog representation (catalog.toProduct). Changing
-- only these does not produce a change. Keep both lists in sync.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_ignored_fields() RETURNS text[]
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT ARRAY['updated_at', 'created_at', 'last_checked_at', 'first_seen_at', 'internal_notes', 'normalized_title',
               'original_title', 'source_evidence', 'extraction_method', 'extraction_confidence', 'captured_page_id',
               'created_by', 'supplier_contact_id', 'company_id']
$$;
-- +goose StatementEnd

-- Raw row-level events, written by triggers inside the writing transaction. catalog.Sequence moves
-- committed events into catalog_changes under an advisory lock, so change IDs follow commit order
-- and a reader of the feed never sees a gap that is filled later.
CREATE TABLE catalog_change_events (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    txid           bigint NOT NULL DEFAULT (pg_current_xact_id()::text::bigint),
    product_id     uuid NOT NULL,
    source         text NOT NULL CHECK (source IN ('product', 'variant', 'image', 'tag', 'tag_name', 'supplier')),
    op             text NOT NULL CHECK (op IN ('INSERT', 'UPDATE', 'DELETE')),
    was_published  boolean NOT NULL,
    is_published   boolean NOT NULL,
    changed_fields text[] NOT NULL DEFAULT '{}',
    ref_id         uuid,
    sku            text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now()
);

-- The ordered change feed of the published catalog, including tombstones.
CREATE TABLE catalog_changes (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    event_id       uuid NOT NULL DEFAULT gen_random_uuid() UNIQUE,
    product_id     uuid NOT NULL,
    kind           text NOT NULL CHECK (kind IN
                   ('published', 'updated', 'price_changed', 'availability_changed', 'unpublished', 'deleted')),
    sku            text NOT NULL DEFAULT '',
    changed_fields text[] NOT NULL DEFAULT '{}',
    -- Only cost, supplier or source fields changed; hidden from clients without catalog.read_costs.
    sourcing_only  boolean NOT NULL DEFAULT false,
    occurred_at    timestamptz NOT NULL,
    sequenced_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX catalog_changes_product_idx ON catalog_changes (product_id, id);
CREATE INDEX catalog_changes_sequenced_idx ON catalog_changes (sequenced_at);

-- Latest change per product: the catalog version used for keyset pagination, ETags and
-- updated_since. Rows of deleted products are removed by the sequencer.
CREATE TABLE catalog_product_state (
    product_id uuid PRIMARY KEY,
    version    bigint NOT NULL,
    changed_at timestamptz NOT NULL,
    published  boolean NOT NULL
);
CREATE INDEX catalog_product_state_version_idx ON catalog_product_state (version);
CREATE INDEX catalog_product_state_changed_idx ON catalog_product_state (changed_at);

-- Highest change ID removed by retention; older feed cursors must resynchronize.
CREATE TABLE catalog_feed_state (
    id             boolean PRIMARY KEY DEFAULT true CHECK (id),
    pruned_through bigint NOT NULL DEFAULT 0
);
INSERT INTO catalog_feed_state DEFAULT VALUES;

-- Products already in the catalog start the feed as "published" changes, so a consumer reading
-- the feed from the beginning receives the whole catalog.
INSERT INTO catalog_changes (product_id, kind, sku, occurred_at)
SELECT id, 'published', sku, updated_at FROM products
WHERE rea_catalog_published(status, review_status)
ORDER BY updated_at, id;
INSERT INTO catalog_product_state (product_id, version, changed_at, published)
SELECT product_id, id, sequenced_at, true FROM catalog_changes;

-- +goose StatementBegin
CREATE FUNCTION rea_catalog_capture_product() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  was     boolean := false;
  now_pub boolean := false;
  fields  text[] := '{}';
BEGIN
  IF TG_OP IN ('UPDATE', 'DELETE') THEN
    was := rea_catalog_published(OLD.status, OLD.review_status);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    now_pub := rea_catalog_published(NEW.status, NEW.review_status);
  END IF;
  IF NOT was AND NOT now_pub THEN
    RETURN NULL; -- not in the catalog before or after this write
  END IF;
  IF TG_OP = 'UPDATE' THEN
    SELECT coalesce(array_agg(n.key ORDER BY n.key), '{}') INTO fields
    FROM jsonb_each(to_jsonb(NEW)) n JOIN jsonb_each(to_jsonb(OLD)) o USING (key)
    WHERE n.value IS DISTINCT FROM o.value AND n.key <> ALL (rea_catalog_ignored_fields());
    IF was = now_pub AND cardinality(fields) = 0 THEN
      RETURN NULL;
    END IF;
    INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, changed_fields, sku)
    VALUES (NEW.id, 'product', 'UPDATE', was, now_pub, fields, NEW.sku);
  ELSIF TG_OP = 'INSERT' THEN
    INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, sku)
    VALUES (NEW.id, 'product', 'INSERT', false, now_pub, NEW.sku);
  ELSE
    INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, sku)
    VALUES (OLD.id, 'product', 'DELETE', was, false, OLD.sku);
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- Records a change of a product's variants, images, tags or supplier, but only while the product
-- is published (a product that becomes published produces a complete "published" change anyway).
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_child_event(p_product uuid, p_source text, p_op text, p_fields text[], p_ref uuid)
RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, changed_fields, ref_id, sku)
  SELECT p.id, p_source, p_op, true, true, p_fields, p_ref, p.sku
  FROM products p
  WHERE p.id = p_product AND rea_catalog_published(p.status, p.review_status);
END;
$$;
-- +goose StatementEnd

-- Variants and images: section name plus "section.column" for each changed column.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_capture_child() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  section text := TG_ARGV[0];
  fields  text[];
BEGIN
  IF TG_OP = 'UPDATE' AND NEW.product_id IS DISTINCT FROM OLD.product_id THEN
    PERFORM rea_catalog_child_event(OLD.product_id, TG_ARGV[1], 'DELETE', ARRAY[section], NULL);
    PERFORM rea_catalog_child_event(NEW.product_id, TG_ARGV[1], 'INSERT', ARRAY[section], NULL);
  ELSIF TG_OP = 'UPDATE' THEN
    SELECT coalesce(array_agg(section || '.' || n.key ORDER BY n.key), '{}') INTO fields
    FROM jsonb_each(to_jsonb(NEW)) n JOIN jsonb_each(to_jsonb(OLD)) o USING (key)
    WHERE n.value IS DISTINCT FROM o.value AND n.key NOT IN ('updated_at', 'created_at');
    IF cardinality(fields) > 0 THEN
      PERFORM rea_catalog_child_event(NEW.product_id, TG_ARGV[1], 'UPDATE', array_prepend(section, fields), NULL);
    END IF;
  ELSIF TG_OP = 'INSERT' THEN
    PERFORM rea_catalog_child_event(NEW.product_id, TG_ARGV[1], 'INSERT', ARRAY[section], NULL);
  ELSE
    PERFORM rea_catalog_child_event(OLD.product_id, TG_ARGV[1], 'DELETE', ARRAY[section], NULL);
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- Tag links record the tag ID so the sequencer can ignore a clear-and-re-add of the same tags.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_capture_tag_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP IN ('UPDATE', 'DELETE') AND OLD.entity_type = 'product' THEN
    PERFORM rea_catalog_child_event(OLD.entity_id, 'tag', 'DELETE', ARRAY['tags'], OLD.tag_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') AND NEW.entity_type = 'product' THEN
    PERFORM rea_catalog_child_event(NEW.entity_id, 'tag', 'INSERT', ARRAY['tags'], NEW.tag_id);
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- A renamed tag changes every published product carrying it.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_capture_tag_rename() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.name::text IS DISTINCT FROM OLD.name::text THEN
    INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, changed_fields, ref_id, sku)
    SELECT p.id, 'tag_name', 'UPDATE', true, true, ARRAY['tags'], NEW.id, p.sku
    FROM record_tags rt JOIN products p ON p.id = rt.entity_id
    WHERE rt.tag_id = NEW.id AND rt.entity_type = 'product' AND rea_catalog_published(p.status, p.review_status);
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

-- A renamed supplier changes the sourcing block of its published products.
-- +goose StatementBegin
CREATE FUNCTION rea_catalog_capture_supplier_rename() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.name IS DISTINCT FROM OLD.name THEN
    INSERT INTO catalog_change_events (product_id, source, op, was_published, is_published, changed_fields, ref_id, sku)
    SELECT p.id, 'supplier', 'UPDATE', true, true, ARRAY['supplier'], NEW.id, p.sku
    FROM products p
    WHERE p.supplier_id = NEW.id AND rea_catalog_published(p.status, p.review_status);
  END IF;
  RETURN NULL;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER products_catalog_capture AFTER INSERT OR UPDATE OR DELETE ON products
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_product();
CREATE TRIGGER product_variants_catalog_capture AFTER INSERT OR UPDATE OR DELETE ON product_variants
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_child('variants', 'variant');
CREATE TRIGGER product_images_catalog_capture AFTER INSERT OR UPDATE OR DELETE ON product_images
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_child('images', 'image');
CREATE TRIGGER record_tags_catalog_capture AFTER INSERT OR UPDATE OR DELETE ON record_tags
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_tag_link();
CREATE TRIGGER tags_catalog_capture AFTER UPDATE OF name ON tags
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_tag_rename();
CREATE TRIGGER suppliers_catalog_capture AFTER UPDATE OF name ON suppliers
    FOR EACH ROW EXECUTE FUNCTION rea_catalog_capture_supplier_rename();

CREATE TABLE webhook_endpoints (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                       text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    url                        text NOT NULL CHECK (length(url) BETWEEN 8 AND 2048),
    events                     text[] NOT NULL CHECK (cardinality(events) >= 1),
    enabled                    boolean NOT NULL DEFAULT true,
    -- Payloads include the sourcing block (cost, supplier, source URL).
    include_costs              boolean NOT NULL DEFAULT false,
    -- HMAC signing secret encrypted with REA_SECRET_KEY (AES-256-GCM); shown once, never returned.
    secret_ciphertext          bytea NOT NULL,
    secret_prefix              text NOT NULL,
    -- After a rotation the previous secret also signs deliveries until it expires.
    previous_secret_ciphertext bytea,
    previous_secret_expires_at timestamptz,
    -- Last catalog_changes.id turned into deliveries for this endpoint.
    change_cursor              bigint NOT NULL DEFAULT 0,
    consecutive_failures       integer NOT NULL DEFAULT 0,
    failing_since              timestamptz,
    last_success_at            timestamptz,
    last_failure_at            timestamptz,
    disabled_reason            text NOT NULL DEFAULT '',
    disabled_at                timestamptz,
    created_by                 uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_deliveries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id     uuid NOT NULL REFERENCES webhook_endpoints(id) ON DELETE CASCADE,
    -- Stable across redeliveries so receivers can deduplicate.
    event_id        uuid NOT NULL,
    event_type      text NOT NULL,
    change_id       bigint,
    product_id      uuid,
    payload         jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    is_test         boolean NOT NULL DEFAULT false,
    status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'delivering', 'succeeded', 'failed')),
    attempts        integer NOT NULL DEFAULT 0,
    max_attempts    integer NOT NULL CHECK (max_attempts BETWEEN 1 AND 50),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_attempt_at timestamptz,
    response_status integer,
    response_body   text NOT NULL DEFAULT '',
    latency_ms      integer,
    error           text NOT NULL DEFAULT '',
    redelivery_of   uuid REFERENCES webhook_deliveries(id) ON DELETE SET NULL,
    locked_by       text NOT NULL DEFAULT '',
    locked_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    finished_at     timestamptz
);
CREATE INDEX webhook_deliveries_endpoint_idx ON webhook_deliveries (endpoint_id, created_at DESC);
CREATE INDEX webhook_deliveries_due_idx ON webhook_deliveries (next_attempt_at) WHERE status = 'pending';
CREATE INDEX webhook_deliveries_running_idx ON webhook_deliveries (locked_at) WHERE status = 'delivering';
CREATE INDEX webhook_deliveries_finished_idx ON webhook_deliveries (finished_at) WHERE finished_at IS NOT NULL;

CREATE TABLE webhook_delivery_attempts (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    delivery_id   uuid NOT NULL REFERENCES webhook_deliveries(id) ON DELETE CASCADE,
    attempt       integer NOT NULL,
    attempted_at  timestamptz NOT NULL DEFAULT now(),
    status_code   integer,
    latency_ms    integer NOT NULL,
    error         text NOT NULL DEFAULT '',
    response_body text NOT NULL DEFAULT ''
);
CREATE INDEX webhook_delivery_attempts_delivery_idx ON webhook_delivery_attempts (delivery_id, attempt);

-- +goose Down
DROP TABLE webhook_delivery_attempts;
DROP TABLE webhook_deliveries;
DROP TABLE webhook_endpoints;
DROP TRIGGER suppliers_catalog_capture ON suppliers;
DROP TRIGGER tags_catalog_capture ON tags;
DROP TRIGGER record_tags_catalog_capture ON record_tags;
DROP TRIGGER product_images_catalog_capture ON product_images;
DROP TRIGGER product_variants_catalog_capture ON product_variants;
DROP TRIGGER products_catalog_capture ON products;
DROP FUNCTION rea_catalog_capture_supplier_rename();
DROP FUNCTION rea_catalog_capture_tag_rename();
DROP FUNCTION rea_catalog_capture_tag_link();
DROP FUNCTION rea_catalog_capture_child();
DROP FUNCTION rea_catalog_child_event(uuid, text, text, text[], uuid);
DROP FUNCTION rea_catalog_capture_product();
DROP TABLE catalog_feed_state;
DROP TABLE catalog_product_state;
DROP TABLE catalog_changes;
DROP TABLE catalog_change_events;
DROP FUNCTION rea_catalog_ignored_fields();
DROP FUNCTION rea_catalog_published(text, text);
DROP INDEX audit_logs_integration_key_idx;
ALTER TABLE audit_logs DROP COLUMN actor_integration_key_id;
DROP TABLE integration_key_usage;
DROP TABLE integration_keys;
