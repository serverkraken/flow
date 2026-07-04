-- +goose Up
ALTER TABLE nodes ADD COLUMN icon text NOT NULL DEFAULT '';
ALTER TABLE nodes ADD COLUMN logo_ref text NOT NULL DEFAULT '';
CREATE TABLE node_logos (
    node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    mime       TEXT NOT NULL,
    ref        TEXT NOT NULL,
    bytes      BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE node_logos;
ALTER TABLE nodes DROP COLUMN logo_ref;
ALTER TABLE nodes DROP COLUMN icon;
