-- +goose Up
CREATE TABLE activity (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT        NOT NULL,
    actor_kind  TEXT        NOT NULL,
    actor_ref   TEXT        NOT NULL,
    kind        TEXT        NOT NULL,
    target_ref  TEXT,
    label       TEXT,
    node_ref    TEXT,
    at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX activity_owner_at ON activity (owner_id, at DESC);

-- +goose Down
DROP TABLE activity;
