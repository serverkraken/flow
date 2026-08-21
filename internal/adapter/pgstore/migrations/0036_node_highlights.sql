-- +goose Up
CREATE TABLE node_highlights (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    document_id TEXT NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    node_id     TEXT NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    quote       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX node_highlights_document_idx ON node_highlights (owner_id, document_id);
CREATE INDEX node_highlights_node_idx ON node_highlights (owner_id, node_id);

-- +goose Down
DROP TABLE node_highlights;
