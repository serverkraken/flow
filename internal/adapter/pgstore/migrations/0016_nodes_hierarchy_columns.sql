-- +goose Up
ALTER TABLE nodes ADD COLUMN parent_id   TEXT REFERENCES nodes(id) ON DELETE RESTRICT;
ALTER TABLE nodes ADD COLUMN kind        TEXT NOT NULL DEFAULT 'repo';
ALTER TABLE nodes ADD COLUMN origin_slug TEXT;
ALTER TABLE nodes ADD COLUMN extra       JSONB NOT NULL DEFAULT '{}';
CREATE INDEX nodes_owner  ON nodes (owner_id);
CREATE INDEX nodes_parent ON nodes (parent_id);

-- +goose Down
DROP INDEX IF EXISTS nodes_parent;
DROP INDEX IF EXISTS nodes_owner;
ALTER TABLE nodes DROP COLUMN extra;
ALTER TABLE nodes DROP COLUMN origin_slug;
ALTER TABLE nodes DROP COLUMN kind;
ALTER TABLE nodes DROP COLUMN parent_id;
