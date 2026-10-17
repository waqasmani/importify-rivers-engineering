-- Track human approval of AI output and the grounding check result.

-- +goose Up
ALTER TABLE ai_jobs ADD COLUMN grounding jsonb;
ALTER TABLE ai_jobs ADD COLUMN applied_at timestamptz;
ALTER TABLE ai_jobs ADD COLUMN applied_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE ai_jobs ADD COLUMN apply_result jsonb;

-- +goose Down
ALTER TABLE ai_jobs DROP COLUMN apply_result;
ALTER TABLE ai_jobs DROP COLUMN applied_by;
ALTER TABLE ai_jobs DROP COLUMN applied_at;
ALTER TABLE ai_jobs DROP COLUMN grounding;
