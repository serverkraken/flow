-- +goose Up
ALTER TABLE documents ADD COLUMN priority INTEGER NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE documents DROP COLUMN priority;
