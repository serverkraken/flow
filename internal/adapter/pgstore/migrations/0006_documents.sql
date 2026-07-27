-- +goose Up
CREATE TABLE documents (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id),
    project_id  TEXT REFERENCES projects(id),
    type        TEXT NOT NULL,
    path        TEXT NOT NULL,
    title       TEXT NOT NULL DEFAULT '',
    body        TEXT NOT NULL DEFAULT '',
    tags        TEXT[] NOT NULL DEFAULT '{}',
    doc_date    DATE,
    role        TEXT,
    extra       JSONB NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX documents_owner_project_path
    ON documents (owner_id, coalesce(project_id, ''), path);

-- +goose Down
DROP TABLE documents;
