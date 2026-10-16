-- Foundation: identity, access control, sessions, workspaces, settings, audit, extension credentials.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE EXTENSION IF NOT EXISTS citext;

-- Shared trigger that keeps updated_at current.
-- +goose StatementBegin
CREATE FUNCTION rea_touch_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;
-- +goose StatementEnd

CREATE TABLE permissions (
    code        text PRIMARY KEY,
    description text NOT NULL
);

CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key         text NOT NULL UNIQUE CHECK (key ~ '^[a-z][a-z0-9_]{1,40}$'),
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    is_system   boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER roles_touch BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE role_permissions (
    role_id         uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    permission_code text NOT NULL REFERENCES permissions(code) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE users (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email               citext NOT NULL UNIQUE CHECK (length(email) BETWEEN 3 AND 320),
    display_name        text NOT NULL CHECK (length(display_name) BETWEEN 1 AND 120),
    password_hash       text NOT NULL,
    role_id             uuid NOT NULL REFERENCES roles(id),
    is_owner            boolean NOT NULL DEFAULT false,
    is_active           boolean NOT NULL DEFAULT true,
    failed_login_count  integer NOT NULL DEFAULT 0,
    locked_until        timestamptz,
    last_login_at       timestamptz,
    password_changed_at timestamptz NOT NULL DEFAULT now(),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
-- Exactly one owner can exist.
CREATE UNIQUE INDEX users_single_owner ON users (is_owner) WHERE is_owner;
CREATE INDEX users_role_idx ON users (role_id);
CREATE TRIGGER users_touch BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE user_sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,
    csrf_token   text NOT NULL,
    ip_address   text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz
);
CREATE INDEX user_sessions_user_active_idx ON user_sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX user_sessions_expires_idx ON user_sessions (expires_at);

CREATE TABLE workspaces (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    slug        text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,62}$'),
    description text NOT NULL DEFAULT '',
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER workspaces_touch BEFORE UPDATE ON workspaces FOR EACH ROW EXECUTE FUNCTION rea_touch_updated_at();

CREATE TABLE app_settings (
    key        text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_.]{1,80}$'),
    value      jsonb NOT NULL,
    updated_by uuid REFERENCES users(id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE extension_devices (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    browser           text NOT NULL DEFAULT '',
    extension_version text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now(),
    last_seen_at      timestamptz,
    last_ip           text NOT NULL DEFAULT '',
    revoked_at        timestamptz
);
CREATE INDEX extension_devices_user_idx ON extension_devices (user_id);

CREATE TABLE extension_pairing_codes (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX extension_pairing_codes_user_idx ON extension_pairing_codes (user_id);

CREATE TABLE api_credentials (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id    uuid REFERENCES extension_devices(id) ON DELETE CASCADE,
    name         text NOT NULL,
    kind         text NOT NULL CHECK (kind IN ('access', 'refresh', 'personal')),
    token_hash   bytea NOT NULL UNIQUE,
    token_prefix text NOT NULL,
    scopes       text[] NOT NULL DEFAULT '{}',
    expires_at   timestamptz NOT NULL,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    revoked_at   timestamptz
);
CREATE INDEX api_credentials_user_idx ON api_credentials (user_id);
CREATE INDEX api_credentials_device_idx ON api_credentials (device_id);
CREATE INDEX api_credentials_expires_idx ON api_credentials (expires_at);

CREATE TABLE audit_logs (
    id              bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    actor_user_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    actor_device_id uuid REFERENCES extension_devices(id) ON DELETE SET NULL,
    actor_label     text NOT NULL DEFAULT '',
    action          text NOT NULL,
    entity_type     text NOT NULL DEFAULT '',
    entity_id       uuid,
    summary         text NOT NULL DEFAULT '',
    changes         jsonb,
    ip_address      text NOT NULL DEFAULT '',
    request_id      text NOT NULL DEFAULT ''
);
CREATE INDEX audit_logs_entity_idx ON audit_logs (entity_type, entity_id, occurred_at DESC);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id, occurred_at DESC);
CREATE INDEX audit_logs_time_idx ON audit_logs (occurred_at DESC);
CREATE INDEX audit_logs_action_idx ON audit_logs (action);

-- Seed permissions and system roles.
INSERT INTO permissions (code, description) VALUES
    ('crm.read',        'View companies, contacts, suppliers, leads, opportunities, tasks and notes'),
    ('crm.write',       'Create and edit CRM records'),
    ('crm.delete',      'Delete and merge CRM records'),
    ('products.read',   'View products, imports and captured pages'),
    ('products.write',  'Create and edit products and imports'),
    ('products.delete', 'Delete and merge products'),
    ('imports.review',  'Approve or reject product imports'),
    ('captures.write',  'Submit captured pages from the browser extension'),
    ('analysis.read',   'View analysis projects, findings and reports'),
    ('analysis.write',  'Create analysis projects, add sources and run analyzers'),
    ('analysis.delete', 'Delete analysis projects and their data'),
    ('ai.use',          'Run AI tools with configured providers'),
    ('ai.configure',    'Configure AI providers'),
    ('data.import',     'Import CSV data'),
    ('data.export',     'Export data as CSV or JSON'),
    ('audit.read',      'View the audit log'),
    ('users.manage',    'Manage users and roles'),
    ('settings.manage', 'Manage system settings, pipelines, custom fields and adapters'),
    ('backups.manage',  'Create and download database backups'),
    ('extension.pair',  'Pair browser extension devices');

INSERT INTO roles (key, name, description, is_system) VALUES
    ('admin',   'Administrator', 'Full access to every feature and setting', true),
    ('analyst', 'Analyst',       'Research, analysis, CRM and product work, including import review', true),
    ('member',  'Member',        'Day-to-day CRM and product capture work', true),
    ('viewer',  'Viewer',        'Read-only access', true);

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r CROSS JOIN permissions p WHERE r.key = 'admin';

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN permissions p ON p.code IN (
    'crm.read','crm.write','crm.delete','products.read','products.write','products.delete','imports.review',
    'captures.write','analysis.read','analysis.write','analysis.delete','ai.use','data.import','data.export','extension.pair'
) WHERE r.key = 'analyst';

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN permissions p ON p.code IN (
    'crm.read','crm.write','products.read','products.write','captures.write','analysis.read','analysis.write','ai.use','extension.pair'
) WHERE r.key = 'member';

INSERT INTO role_permissions (role_id, permission_code)
SELECT r.id, p.code FROM roles r JOIN permissions p ON p.code IN (
    'crm.read','products.read','analysis.read'
) WHERE r.key = 'viewer';

-- +goose Down
DROP TABLE audit_logs;
DROP TABLE api_credentials;
DROP TABLE extension_pairing_codes;
DROP TABLE extension_devices;
DROP TABLE app_settings;
DROP TABLE workspaces;
DROP TABLE user_sessions;
DROP TABLE users;
DROP TABLE role_permissions;
DROP TABLE roles;
DROP TABLE permissions;
DROP FUNCTION rea_touch_updated_at();
DROP EXTENSION IF EXISTS citext;
DROP EXTENSION IF EXISTS pg_trgm;
