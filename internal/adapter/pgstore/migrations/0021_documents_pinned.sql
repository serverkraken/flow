-- +goose Up
ALTER TABLE documents ADD COLUMN pinned BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE documents DROP COLUMN pinned;
