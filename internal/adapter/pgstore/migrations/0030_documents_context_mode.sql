-- +goose Up
ALTER TABLE documents ADD COLUMN context_mode TEXT NOT NULL DEFAULT 'auto'
    CHECK (context_mode IN ('auto','immer','nie'));

-- +goose Down
ALTER TABLE documents DROP COLUMN context_mode;
