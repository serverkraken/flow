-- +goose Up
CREATE TABLE project_bindings (
  id            TEXT PRIMARY KEY,
  owner_id      TEXT NOT NULL,
  project_id    TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL CHECK (kind IN ('remote','path')),
  remote_slug   TEXT,
  machine_id    TEXT,
  machine_label TEXT,
  path          TEXT,
  created_at    TIMESTAMPTZ NOT NULL,
  updated_at    TIMESTAMPTZ NOT NULL
);
CREATE UNIQUE INDEX project_bindings_remote ON project_bindings (owner_id, remote_slug) WHERE kind='remote';
CREATE UNIQUE INDEX project_bindings_path   ON project_bindings (owner_id, machine_id, path) WHERE kind='path';
CREATE INDEX project_bindings_owner ON project_bindings (owner_id);

-- +goose Down
DROP TABLE IF EXISTS project_bindings;
