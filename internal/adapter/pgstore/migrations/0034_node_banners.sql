-- +goose Up
ALTER TABLE nodes ADD COLUMN banner_ref text NOT NULL DEFAULT '';
CREATE TABLE node_banners (
    node_id    TEXT PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    mime       TEXT NOT NULL,
    ref        TEXT NOT NULL,
    bytes      BYTEA NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE node_banners;
ALTER TABLE nodes DROP COLUMN banner_ref;
