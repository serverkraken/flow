-- +goose Up
ALTER TABLE projects ADD COLUMN description  TEXT NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN upstream_git TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE projects DROP COLUMN upstream_git;
ALTER TABLE projects DROP COLUMN description;
