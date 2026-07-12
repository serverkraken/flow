-- +goose Up
CREATE TABLE artifacts (
    id              TEXT PRIMARY KEY,
    owner_id        TEXT NOT NULL REFERENCES users(id),
    node_id         TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    mime            TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL,
    ref             TEXT NOT NULL,
    bytes           BYTEA NOT NULL,
    width           INTEGER,
    height          INTEGER,
    created_by_kind TEXT NOT NULL DEFAULT '',
    created_by_ref  TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL,
    UNIQUE (owner_id, node_id, slug)
);
CREATE INDEX artifacts_owner_node ON artifacts (owner_id, node_id);

-- +goose Down
DROP TABLE artifacts;
