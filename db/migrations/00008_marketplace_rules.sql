-- Declarative "rules" adapters: marketplace definitions shipped with REA (db/marketplaces) and
-- custom rule documents created by administrators.
-- +goose Up
ALTER TABLE source_adapters DROP CONSTRAINT source_adapters_kind_check;
ALTER TABLE source_adapters ADD CONSTRAINT source_adapters_kind_check CHECK (kind IN ('builtin', 'selector', 'rules'));

ALTER TABLE source_adapters
    -- Rule document for kind = 'rules' (validated by the API and again by the extractor).
    ADD COLUMN rules jsonb CHECK (rules IS NULL OR jsonb_typeof(rules) = 'object'),
    -- Shipped with REA: built-in code adapters and marketplace definitions. Not deletable; their
    -- definition is maintained with REA, administrators only switch them on or off and re-prioritise.
    ADD COLUMN shipped boolean NOT NULL DEFAULT false,
    -- Version of the shipped definition; a newer shipped version replaces the stored one at start-up.
    ADD COLUMN definition_version integer NOT NULL DEFAULT 0 CHECK (definition_version >= 0),
    ADD COLUMN region text NOT NULL DEFAULT '' CHECK (length(region) <= 40),
    -- How the definition was checked, e.g. {"status": "synthetic_fixture", "note": "..."}.
    ADD COLUMN verification jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(verification) = 'object');

ALTER TABLE source_adapters ADD CONSTRAINT source_adapters_rules_kind CHECK ((kind = 'rules') = (rules IS NOT NULL));

UPDATE source_adapters SET shipped = true WHERE kind = 'builtin';

-- +goose Down
DELETE FROM source_adapters WHERE kind = 'rules';
ALTER TABLE source_adapters DROP CONSTRAINT source_adapters_rules_kind;
ALTER TABLE source_adapters
    DROP COLUMN verification,
    DROP COLUMN region,
    DROP COLUMN definition_version,
    DROP COLUMN shipped,
    DROP COLUMN rules;
ALTER TABLE source_adapters DROP CONSTRAINT source_adapters_kind_check;
ALTER TABLE source_adapters ADD CONSTRAINT source_adapters_kind_check CHECK (kind IN ('builtin', 'selector'));
