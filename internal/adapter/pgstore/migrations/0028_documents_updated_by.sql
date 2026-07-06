-- +goose Up
ALTER TABLE documents ADD COLUMN updated_by_kind text;
ALTER TABLE documents ADD COLUMN updated_by_ref text;

-- +goose Down
ALTER TABLE documents DROP COLUMN updated_by_ref;
ALTER TABLE documents DROP COLUMN updated_by_kind;
