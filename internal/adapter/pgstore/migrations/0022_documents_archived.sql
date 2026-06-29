-- +goose Up
ALTER TABLE documents ADD COLUMN archived    BOOLEAN     NOT NULL DEFAULT false;
ALTER TABLE documents ADD COLUMN archived_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE documents DROP COLUMN archived_at;
ALTER TABLE documents DROP COLUMN archived;
