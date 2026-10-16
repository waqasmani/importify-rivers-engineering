-- CRM: pipelines, companies, contacts, suppliers, leads, opportunities, tasks, notes, attachments,
-- tags, custom fields and saved filters.

-- +goose Up

-- Full-text document helper. It is IMMUTABLE so it can back expression indexes; queries must call it
-- with the same column list as the index to use it.
-- +goose StatementBegin
CREATE FUNCTION rea_search_doc(a text, b text, c text, d text) RETURNS tsvector
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT to_tsvector('simple'::regconfig,
    coalesce(a, '') || ' ' || coalesce(b, '') || ' ' || coalesce(c, '') || ' ' || coalesce(d, ''))
$$;
-- +goose StatementEnd

CREATE TABLE pipelines (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL CHECK (entity_type IN ('lead', 'opportunity', 'supplier')),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    is_default  boolean NOT NULL DEFAULT false,
    position    integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_type, name)
);
CREATE UNIQUE INDEX pipelines_one_default ON pipelines (entity_type) WHERE is_default;
CREATE TRIGGER pipelines_touch BEFORE UPDATE ON pipelines FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE pipeline_stages (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    pipeline_id uuid NOT NULL REFERENCES pipelines(id) ON DELETE CASCADE,
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    position    integer NOT NULL DEFAULT 0,
    category    text NOT NULL DEFAULT 'open' CHECK (category IN ('open', 'won', 'lost')),
    color       text NOT NULL DEFAULT '#64748b' CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
    probability smallint CHECK (probability BETWEEN 0 AND 100),
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (pipeline_id, name)
);
CREATE INDEX pipeline_stages_pipeline_idx ON pipeline_stages (pipeline_id, position);

CREATE TABLE companies (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    normalized_name text NOT NULL,
    domain          text,
    website         text,
    industry        text NOT NULL DEFAULT '',
    company_type    text NOT NULL DEFAULT 'other' CHECK (company_type IN
                    ('customer', 'prospect', 'partner', 'competitor', 'supplier', 'manufacturer', 'research_target', 'other')),
    status          text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'archived')),
    country         text NOT NULL DEFAULT '',
    city            text NOT NULL DEFAULT '',
    address         text NOT NULL DEFAULT '',
    phone           text NOT NULL DEFAULT '',
    email           text NOT NULL DEFAULT '',
    description     text NOT NULL DEFAULT '',
    employee_count  integer CHECK (employee_count >= 0),
    owner_id        uuid REFERENCES users(id) ON DELETE SET NULL,
    custom_fields   jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX companies_search_idx ON companies USING gin (rea_search_doc(name, domain, industry, description));
CREATE INDEX companies_name_trgm_idx ON companies USING gin (normalized_name gin_trgm_ops);
CREATE INDEX companies_domain_idx ON companies (domain) WHERE domain IS NOT NULL;
CREATE INDEX companies_owner_idx ON companies (owner_id);
CREATE INDEX companies_updated_idx ON companies (updated_at DESC);
CREATE TRIGGER companies_touch BEFORE UPDATE ON companies FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE contacts (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    first_name        text NOT NULL DEFAULT '',
    last_name         text NOT NULL DEFAULT '',
    full_name         text GENERATED ALWAYS AS (btrim(first_name || ' ' || last_name)) STORED,
    email             citext,
    phone             text NOT NULL DEFAULT '',
    job_title         text NOT NULL DEFAULT '',
    company_id        uuid REFERENCES companies(id) ON DELETE SET NULL,
    is_decision_maker boolean NOT NULL DEFAULT false,
    linkedin_url      text NOT NULL DEFAULT '',
    country           text NOT NULL DEFAULT '',
    status            text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive', 'archived')),
    notes_summary     text NOT NULL DEFAULT '',
    owner_id          uuid REFERENCES users(id) ON DELETE SET NULL,
    custom_fields     jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CHECK (length(first_name) + length(last_name) BETWEEN 1 AND 200)
);
CREATE INDEX contacts_search_idx ON contacts USING gin (rea_search_doc(first_name, last_name, email::text, job_title));
CREATE INDEX contacts_name_trgm_idx ON contacts USING gin (lower(full_name) gin_trgm_ops);
CREATE INDEX contacts_email_idx ON contacts (email) WHERE email IS NOT NULL;
CREATE INDEX contacts_company_idx ON contacts (company_id);
CREATE INDEX contacts_owner_idx ON contacts (owner_id);
CREATE INDEX contacts_updated_idx ON contacts (updated_at DESC);
CREATE TRIGGER contacts_touch BEFORE UPDATE ON contacts FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE suppliers (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    normalized_name    text NOT NULL,
    company_id         uuid REFERENCES companies(id) ON DELETE SET NULL,
    supplier_type      text NOT NULL DEFAULT 'other' CHECK (supplier_type IN
                       ('manufacturer', 'wholesaler', 'distributor', 'trading_company', 'dropshipper', 'service_provider', 'other')),
    website            text,
    domain             text,
    country            text NOT NULL DEFAULT '',
    city               text NOT NULL DEFAULT '',
    address            text NOT NULL DEFAULT '',
    email              text NOT NULL DEFAULT '',
    phone              text NOT NULL DEFAULT '',
    primary_contact_id uuid REFERENCES contacts(id) ON DELETE SET NULL,
    pipeline_stage_id  uuid REFERENCES pipeline_stages(id) ON DELETE SET NULL,
    rating             smallint CHECK (rating BETWEEN 1 AND 5),
    payment_terms      text NOT NULL DEFAULT '',
    lead_time_days     integer CHECK (lead_time_days >= 0),
    moq_notes          text NOT NULL DEFAULT '',
    certifications     text[] NOT NULL DEFAULT '{}',
    description        text NOT NULL DEFAULT '',
    owner_id           uuid REFERENCES users(id) ON DELETE SET NULL,
    custom_fields      jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by         uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX suppliers_search_idx ON suppliers USING gin (rea_search_doc(name, domain, country, description));
CREATE INDEX suppliers_name_trgm_idx ON suppliers USING gin (normalized_name gin_trgm_ops);
CREATE INDEX suppliers_domain_idx ON suppliers (domain) WHERE domain IS NOT NULL;
CREATE INDEX suppliers_company_idx ON suppliers (company_id);
CREATE INDEX suppliers_stage_idx ON suppliers (pipeline_stage_id);
CREATE INDEX suppliers_updated_idx ON suppliers (updated_at DESC);
CREATE TRIGGER suppliers_touch BEFORE UPDATE ON suppliers FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- A contact may represent a supplier (supplier contact) as well as a company.
ALTER TABLE contacts ADD COLUMN supplier_id uuid REFERENCES suppliers(id) ON DELETE SET NULL;
CREATE INDEX contacts_supplier_idx ON contacts (supplier_id);

CREATE TABLE leads (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title             text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    company_id        uuid REFERENCES companies(id) ON DELETE SET NULL,
    contact_id        uuid REFERENCES contacts(id) ON DELETE SET NULL,
    source            text NOT NULL DEFAULT 'other' CHECK (source IN
                      ('website', 'referral', 'extension', 'event', 'import', 'research', 'inbound', 'outbound', 'other')),
    pipeline_stage_id uuid REFERENCES pipeline_stages(id) ON DELETE SET NULL,
    estimated_value   numeric(14, 2) CHECK (estimated_value >= 0),
    currency          char(3) NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
    description       text NOT NULL DEFAULT '',
    owner_id          uuid REFERENCES users(id) ON DELETE SET NULL,
    converted_at      timestamptz,
    custom_fields     jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by        uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX leads_search_idx ON leads USING gin (rea_search_doc(title, source, description, ''));
CREATE INDEX leads_stage_idx ON leads (pipeline_stage_id);
CREATE INDEX leads_company_idx ON leads (company_id);
CREATE INDEX leads_owner_idx ON leads (owner_id);
CREATE INDEX leads_updated_idx ON leads (updated_at DESC);
CREATE TRIGGER leads_touch BEFORE UPDATE ON leads FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE opportunities (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    company_id          uuid REFERENCES companies(id) ON DELETE SET NULL,
    supplier_id         uuid REFERENCES suppliers(id) ON DELETE SET NULL,
    contact_id          uuid REFERENCES contacts(id) ON DELETE SET NULL,
    lead_id             uuid REFERENCES leads(id) ON DELETE SET NULL,
    pipeline_stage_id   uuid REFERENCES pipeline_stages(id) ON DELETE SET NULL,
    amount              numeric(14, 2) CHECK (amount >= 0),
    currency            char(3) NOT NULL DEFAULT 'USD' CHECK (currency ~ '^[A-Z]{3}$'),
    expected_close_date date,
    probability         smallint CHECK (probability BETWEEN 0 AND 100),
    status              text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'won', 'lost')),
    closed_at           timestamptz,
    outcome_reason      text NOT NULL DEFAULT '',
    description         text NOT NULL DEFAULT '',
    owner_id            uuid REFERENCES users(id) ON DELETE SET NULL,
    custom_fields       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(custom_fields) = 'object'),
    created_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX opportunities_search_idx ON opportunities USING gin (rea_search_doc(name, description, outcome_reason, ''));
CREATE INDEX opportunities_stage_idx ON opportunities (pipeline_stage_id);
CREATE INDEX opportunities_status_idx ON opportunities (status, expected_close_date);
CREATE INDEX opportunities_company_idx ON opportunities (company_id);
CREATE INDEX opportunities_supplier_idx ON opportunities (supplier_id);
CREATE INDEX opportunities_owner_idx ON opportunities (owner_id);
CREATE INDEX opportunities_updated_idx ON opportunities (updated_at DESC);
CREATE TRIGGER opportunities_touch BEFORE UPDATE ON opportunities FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

ALTER TABLE leads ADD COLUMN converted_opportunity_id uuid REFERENCES opportunities(id) ON DELETE SET NULL;

-- Records that notes, tasks, tags and attachments can be attached to.
-- +goose StatementBegin
CREATE FUNCTION rea_valid_entity_type(t text) RETURNS boolean LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
  SELECT t IN ('company', 'contact', 'supplier', 'lead', 'opportunity', 'product', 'analysis_project', 'analysis_source', 'captured_page')
$$;
-- +goose StatementEnd

CREATE TABLE tasks (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    title        text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
    description  text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_progress', 'done', 'cancelled')),
    priority     text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low', 'normal', 'high', 'urgent')),
    task_type    text NOT NULL DEFAULT 'todo' CHECK (task_type IN ('todo', 'follow_up', 'call', 'email', 'meeting', 'review')),
    due_at       timestamptz,
    remind_at    timestamptz,
    completed_at timestamptz,
    assignee_id  uuid REFERENCES users(id) ON DELETE SET NULL,
    entity_type  text CHECK (entity_type IS NULL OR rea_valid_entity_type(entity_type)),
    entity_id    uuid,
    created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((entity_type IS NULL) = (entity_id IS NULL))
);
CREATE INDEX tasks_assignee_idx ON tasks (assignee_id, status, due_at);
CREATE INDEX tasks_entity_idx ON tasks (entity_type, entity_id);
CREATE INDEX tasks_open_due_idx ON tasks (due_at) WHERE status IN ('open', 'in_progress');
CREATE INDEX tasks_search_idx ON tasks USING gin (rea_search_doc(title, description, '', ''));
CREATE TRIGGER tasks_touch BEFORE UPDATE ON tasks FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE notes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL CHECK (rea_valid_entity_type(entity_type)),
    entity_id   uuid NOT NULL,
    parent_id   uuid REFERENCES notes(id) ON DELETE CASCADE,
    body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 50000),
    is_pinned   boolean NOT NULL DEFAULT false,
    author_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notes_entity_idx ON notes (entity_type, entity_id, created_at DESC);
CREATE INDEX notes_parent_idx ON notes (parent_id);
CREATE TRIGGER notes_touch BEFORE UPDATE ON notes FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE attachments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type  text CHECK (entity_type IS NULL OR rea_valid_entity_type(entity_type)),
    entity_id    uuid,
    filename     text NOT NULL CHECK (length(filename) BETWEEN 1 AND 255),
    content_type text NOT NULL,
    size_bytes   bigint NOT NULL CHECK (size_bytes >= 0),
    sha256       text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    storage_key  text NOT NULL,
    uploaded_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    CHECK ((entity_type IS NULL) = (entity_id IS NULL))
);
CREATE INDEX attachments_entity_idx ON attachments (entity_type, entity_id);
CREATE INDEX attachments_storage_idx ON attachments (storage_key);

CREATE TABLE tags (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       citext NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 60),
    color      text NOT NULL DEFAULT '#6366f1' CHECK (color ~ '^#[0-9a-fA-F]{6}$'),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE record_tags (
    tag_id      uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    entity_type text NOT NULL CHECK (rea_valid_entity_type(entity_type)),
    entity_id   uuid NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tag_id, entity_type, entity_id)
);
CREATE INDEX record_tags_entity_idx ON record_tags (entity_type, entity_id);

CREATE TABLE custom_field_definitions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type text NOT NULL CHECK (entity_type IN ('company', 'contact', 'supplier', 'lead', 'opportunity', 'product')),
    key         text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{0,40}$'),
    label       text NOT NULL CHECK (length(label) BETWEEN 1 AND 80),
    field_type  text NOT NULL CHECK (field_type IN ('text', 'number', 'date', 'boolean', 'select', 'url')),
    options     text[] NOT NULL DEFAULT '{}',
    required    boolean NOT NULL DEFAULT false,
    position    integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_type, key)
);
CREATE TRIGGER custom_field_definitions_touch BEFORE UPDATE ON custom_field_definitions FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE saved_filters (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type text NOT NULL CHECK (rea_valid_entity_type(entity_type) OR entity_type IN ('task', 'product_import')),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
    filters     jsonb NOT NULL CHECK (jsonb_typeof(filters) = 'object'),
    is_shared   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX saved_filters_user_idx ON saved_filters (user_id, entity_type);
CREATE TRIGGER saved_filters_touch BEFORE UPDATE ON saved_filters FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

-- Remove polymorphic children when a parent record is deleted. Tasks are kept but unlinked so
-- follow-up history is not lost.
-- +goose StatementBegin
CREATE FUNCTION rea_cleanup_record_children() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  DELETE FROM notes WHERE entity_type = TG_ARGV[0] AND entity_id = OLD.id;
  DELETE FROM record_tags WHERE entity_type = TG_ARGV[0] AND entity_id = OLD.id;
  DELETE FROM attachments WHERE entity_type = TG_ARGV[0] AND entity_id = OLD.id;
  UPDATE tasks SET entity_type = NULL, entity_id = NULL WHERE entity_type = TG_ARGV[0] AND entity_id = OLD.id;
  RETURN OLD;
END;
$$;
-- +goose StatementEnd

CREATE TRIGGER companies_cleanup AFTER DELETE ON companies FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('company');
CREATE TRIGGER contacts_cleanup AFTER DELETE ON contacts FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('contact');
CREATE TRIGGER suppliers_cleanup AFTER DELETE ON suppliers FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('supplier');
CREATE TRIGGER leads_cleanup AFTER DELETE ON leads FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('lead');
CREATE TRIGGER opportunities_cleanup AFTER DELETE ON opportunities FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('opportunity');

-- Default pipelines. Operators can rename, reorder or replace these stages.
-- +goose StatementBegin
DO $$
DECLARE pid uuid;
BEGIN
  INSERT INTO pipelines (entity_type, name, is_default) VALUES ('lead', 'Lead qualification', true) RETURNING id INTO pid;
  INSERT INTO pipeline_stages (pipeline_id, name, position, category, color) VALUES
    (pid, 'New', 0, 'open', '#64748b'), (pid, 'Contacted', 1, 'open', '#0ea5e9'),
    (pid, 'Qualified', 2, 'open', '#8b5cf6'), (pid, 'Converted', 3, 'won', '#16a34a'),
    (pid, 'Disqualified', 4, 'lost', '#dc2626');

  INSERT INTO pipelines (entity_type, name, is_default) VALUES ('opportunity', 'Sourcing deals', true) RETURNING id INTO pid;
  INSERT INTO pipeline_stages (pipeline_id, name, position, category, color, probability) VALUES
    (pid, 'Research', 0, 'open', '#64748b', 10), (pid, 'Quotation requested', 1, 'open', '#0ea5e9', 25),
    (pid, 'Quotation received', 2, 'open', '#6366f1', 40), (pid, 'Negotiation', 3, 'open', '#f59e0b', 60),
    (pid, 'Sample ordered', 4, 'open', '#8b5cf6', 75), (pid, 'Won', 5, 'won', '#16a34a', 100),
    (pid, 'Lost', 6, 'lost', '#dc2626', 0);

  INSERT INTO pipelines (entity_type, name, is_default) VALUES ('supplier', 'Supplier qualification', true) RETURNING id INTO pid;
  INSERT INTO pipeline_stages (pipeline_id, name, position, category, color) VALUES
    (pid, 'Discovered', 0, 'open', '#64748b'), (pid, 'Contacted', 1, 'open', '#0ea5e9'),
    (pid, 'Evaluating', 2, 'open', '#f59e0b'), (pid, 'Qualified', 3, 'open', '#8b5cf6'),
    (pid, 'Approved', 4, 'won', '#16a34a'), (pid, 'Rejected', 5, 'lost', '#dc2626');
END;
$$;
-- +goose StatementEnd

-- +goose Down
DROP TRIGGER opportunities_cleanup ON opportunities;
DROP TRIGGER leads_cleanup ON leads;
DROP TRIGGER suppliers_cleanup ON suppliers;
DROP TRIGGER contacts_cleanup ON contacts;
DROP TRIGGER companies_cleanup ON companies;
DROP FUNCTION rea_cleanup_record_children();
DROP TABLE saved_filters;
DROP TABLE custom_field_definitions;
DROP TABLE record_tags;
DROP TABLE tags;
DROP TABLE attachments;
DROP TABLE notes;
DROP TABLE tasks;
DROP FUNCTION rea_valid_entity_type(text);
ALTER TABLE leads DROP COLUMN converted_opportunity_id;
DROP TABLE opportunities;
DROP TABLE leads;
ALTER TABLE contacts DROP COLUMN supplier_id;
DROP TABLE suppliers;
DROP TABLE contacts;
DROP TABLE companies;
DROP TABLE pipeline_stages;
DROP TABLE pipelines;
DROP FUNCTION rea_search_doc(text, text, text, text);
