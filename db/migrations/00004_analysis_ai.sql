-- REA analysis engine and AI orchestration.

-- +goose Up

CREATE TABLE ai_provider_configs (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 80),
    kind                text NOT NULL CHECK (kind IN ('ollama', 'openai', 'openai_compatible', 'anthropic')),
    base_url            text NOT NULL CHECK (length(base_url) <= 2048),
    model               text NOT NULL CHECK (length(model) BETWEEN 1 AND 200),
    -- API key encrypted with AES-256-GCM using REA_SECRET_KEY. Never returned by the API.
    api_key_ciphertext  bytea,
    is_default          boolean NOT NULL DEFAULT false,
    enabled             boolean NOT NULL DEFAULT true,
    -- True when the endpoint is on this machine or a private network.
    is_local            boolean NOT NULL DEFAULT false,
    -- An administrator explicitly allowed sending stored data to this (external) provider.
    allow_external_data boolean NOT NULL DEFAULT false,
    supports_vision     boolean NOT NULL DEFAULT false,
    temperature         numeric(3, 2) NOT NULL DEFAULT 0.2 CHECK (temperature BETWEEN 0 AND 2),
    max_output_tokens   integer NOT NULL DEFAULT 2048 CHECK (max_output_tokens BETWEEN 64 AND 200000),
    timeout_seconds     integer NOT NULL DEFAULT 120 CHECK (timeout_seconds BETWEEN 5 AND 1800),
    created_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ai_provider_configs_one_default ON ai_provider_configs (is_default) WHERE is_default;
CREATE TRIGGER ai_provider_configs_touch BEFORE UPDATE ON ai_provider_configs FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE ai_jobs (
    id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tool                    text NOT NULL,
    status                  text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    provider_config_id      uuid REFERENCES ai_provider_configs(id) ON DELETE SET NULL,
    provider_kind           text NOT NULL DEFAULT '',
    provider_name           text NOT NULL DEFAULT '',
    model                   text NOT NULL DEFAULT '',
    entity_type             text NOT NULL DEFAULT '',
    entity_id               uuid,
    input                   jsonb NOT NULL DEFAULT '{}',
    -- Evidence items supplied to the model: [{"ref":"E1","type":"finding","id":"...","label":"..."}].
    evidence_refs           jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(evidence_refs) = 'array'),
    output                  jsonb,
    output_text             text NOT NULL DEFAULT '',
    error                   text NOT NULL DEFAULT '',
    attempts                integer NOT NULL DEFAULT 0,
    max_attempts            integer NOT NULL DEFAULT 2,
    input_tokens            integer,
    output_tokens           integer,
    duration_ms             integer,
    external_data_confirmed boolean NOT NULL DEFAULT false,
    requested_by            uuid REFERENCES users(id) ON DELETE SET NULL,
    queued_at               timestamptz NOT NULL DEFAULT now(),
    started_at              timestamptz,
    finished_at             timestamptz,
    locked_by               text NOT NULL DEFAULT '',
    locked_at               timestamptz
);
CREATE INDEX ai_jobs_queue_idx ON ai_jobs (queued_at) WHERE status = 'queued';
CREATE INDEX ai_jobs_entity_idx ON ai_jobs (entity_type, entity_id, queued_at DESC);
CREATE INDEX ai_jobs_user_idx ON ai_jobs (requested_by, queued_at DESC);
CREATE INDEX ai_jobs_running_idx ON ai_jobs (locked_at) WHERE status = 'running';

CREATE TABLE analysis_projects (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id               uuid NOT NULL REFERENCES workspaces(id) ON DELETE RESTRICT,
    name                       text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
    description                text NOT NULL DEFAULT '',
    objective                  text NOT NULL DEFAULT '',
    status                     text NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'completed', 'archived')),
    analysis_types             text[] NOT NULL DEFAULT '{}',
    authorization_basis        text NOT NULL CHECK (authorization_basis IN
                               ('owned', 'client_authorized', 'public_information', 'licensed', 'open_source', 'other')),
    authorization_note         text NOT NULL DEFAULT '',
    authorization_confirmed_by uuid REFERENCES users(id) ON DELETE SET NULL,
    authorization_confirmed_at timestamptz NOT NULL,
    owner_id                   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by                 uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analysis_projects_workspace_idx ON analysis_projects (workspace_id, updated_at DESC);
CREATE INDEX analysis_projects_search_idx ON analysis_projects USING gin (rea_search_doc(name, description, objective, ''));
CREATE TRIGGER analysis_projects_touch BEFORE UPDATE ON analysis_projects FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();
CREATE TRIGGER analysis_projects_cleanup AFTER DELETE ON analysis_projects FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('analysis_project');

CREATE TABLE analysis_sources (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid NOT NULL REFERENCES analysis_projects(id) ON DELETE CASCADE,
    kind             text NOT NULL CHECK (kind IN
                     ('url', 'captured_page', 'html', 'repository_archive', 'repository_path', 'openapi',
                      'api_examples', 'document', 'screenshot', 'text', 'workflow')),
    title            text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
    url              text CHECK (length(url) <= 2048),
    captured_page_id uuid REFERENCES captured_pages(id) ON DELETE SET NULL,
    attachment_id    uuid REFERENCES attachments(id) ON DELETE SET NULL,
    -- Extracted plain text of the source, bounded; the original file stays in attachments.
    content_text     text NOT NULL DEFAULT '' CHECK (length(content_text) <= 5000000),
    content_sha256   text NOT NULL DEFAULT '',
    mime_type        text NOT NULL DEFAULT '',
    size_bytes       bigint NOT NULL DEFAULT 0,
    metadata         jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(metadata) = 'object'),
    status           text NOT NULL DEFAULT 'ready' CHECK (status IN ('pending', 'ready', 'failed')),
    error            text NOT NULL DEFAULT '',
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analysis_sources_project_idx ON analysis_sources (project_id, created_at DESC);
CREATE TRIGGER analysis_sources_cleanup AFTER DELETE ON analysis_sources FOR EACH ROW EXECUTE FUNCTION rea_cleanup_record_children('analysis_source');

CREATE TABLE analysis_runs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid NOT NULL REFERENCES analysis_projects(id) ON DELETE CASCADE,
    source_id   uuid REFERENCES analysis_sources(id) ON DELETE CASCADE,
    analyzer    text NOT NULL,
    status      text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    attempts    integer NOT NULL DEFAULT 0,
    options     jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(options) = 'object'),
    stats       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(stats) = 'object'),
    error       text NOT NULL DEFAULT '',
    ai_job_id   uuid REFERENCES ai_jobs(id) ON DELETE SET NULL,
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    queued_at   timestamptz NOT NULL DEFAULT now(),
    started_at  timestamptz,
    finished_at timestamptz,
    locked_by   text NOT NULL DEFAULT '',
    locked_at   timestamptz
);
CREATE INDEX analysis_runs_project_idx ON analysis_runs (project_id, queued_at DESC);
CREATE INDEX analysis_runs_queue_idx ON analysis_runs (queued_at) WHERE status = 'queued';
CREATE INDEX analysis_runs_running_idx ON analysis_runs (locked_at) WHERE status = 'running';

CREATE TABLE analysis_findings (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid NOT NULL REFERENCES analysis_projects(id) ON DELETE CASCADE,
    run_id           uuid REFERENCES analysis_runs(id) ON DELETE SET NULL,
    source_id        uuid REFERENCES analysis_sources(id) ON DELETE SET NULL,
    category         text NOT NULL CHECK (category ~ '^[a-z][a-z_]{1,40}$'),
    title            text NOT NULL CHECK (length(title) BETWEEN 1 AND 500),
    detail           text NOT NULL DEFAULT '',
    evidence         text NOT NULL DEFAULT '' CHECK (length(evidence) <= 8000),
    evidence_locator text NOT NULL DEFAULT '',
    classification   text NOT NULL CHECK (classification IN ('verified', 'inferred', 'unknown')),
    confidence       numeric(4, 3) NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    assumptions      text NOT NULL DEFAULT '',
    origin           text NOT NULL CHECK (origin IN ('rule', 'ai', 'manual')),
    -- Structured analyzer payload (endpoint, model, dependency details...).
    data             jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(data) = 'object'),
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analysis_findings_project_idx ON analysis_findings (project_id, category);
CREATE INDEX analysis_findings_run_idx ON analysis_findings (run_id);
CREATE INDEX analysis_findings_source_idx ON analysis_findings (source_id);
CREATE INDEX analysis_findings_search_idx ON analysis_findings USING gin (rea_search_doc(title, detail, evidence, category));
CREATE TRIGGER analysis_findings_touch BEFORE UPDATE ON analysis_findings FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE analysis_reports (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid NOT NULL REFERENCES analysis_projects(id) ON DELETE CASCADE,
    kind             text NOT NULL CHECK (kind IN
                     ('summary', 'technical_documentation', 'architecture', 'api_inventory', 'data_model',
                      'workflow', 'reconstruction_plan', 'comparison', 'custom')),
    title            text NOT NULL CHECK (length(title) BETWEEN 1 AND 300),
    content_markdown text NOT NULL,
    origin           text NOT NULL CHECK (origin IN ('rule', 'ai', 'mixed', 'manual')),
    is_ai_generated  boolean NOT NULL DEFAULT false,
    ai_job_id        uuid REFERENCES ai_jobs(id) ON DELETE SET NULL,
    provider_name    text NOT NULL DEFAULT '',
    model            text NOT NULL DEFAULT '',
    finding_ids      uuid[] NOT NULL DEFAULT '{}',
    source_ids       uuid[] NOT NULL DEFAULT '{}',
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX analysis_reports_project_idx ON analysis_reports (project_id, created_at DESC);

-- +goose Down
DROP TABLE analysis_reports;
DROP TABLE analysis_findings;
DROP TABLE analysis_runs;
DROP TABLE analysis_sources;
DROP TABLE analysis_projects;
DROP TABLE ai_jobs;
DROP TABLE ai_provider_configs;
