-- +goose Up
CREATE TABLE users (
    id            TEXT    PRIMARY KEY,
    oidc_sub      TEXT    NOT NULL UNIQUE,
    email         TEXT    NOT NULL DEFAULT '',
    display_name  TEXT    NOT NULL DEFAULT '',
    created_at    TEXT    NOT NULL
);

CREATE TABLE projects (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    name          TEXT    NOT NULL,
    slug          TEXT    NOT NULL,
    created_at    TEXT    NOT NULL,
    last_used_at  TEXT    NOT NULL DEFAULT '',
    archived_at   TEXT,
    version       INTEGER NOT NULL DEFAULT 0,
    UNIQUE(user_id, slug)
);
CREATE INDEX idx_projects_user_last_used ON projects(user_id, last_used_at DESC);

CREATE TABLE sessions (
    id            TEXT    PRIMARY KEY,
    user_id       TEXT    NOT NULL REFERENCES users(id),
    project_id    TEXT    NOT NULL REFERENCES projects(id),
    date          TEXT    NOT NULL,
    start         TEXT    NOT NULL,
    stop          TEXT    NOT NULL,
    elapsed_ns    INTEGER NOT NULL,
    tag           TEXT    NOT NULL DEFAULT '',
    note          TEXT    NOT NULL DEFAULT '',
    version       INTEGER NOT NULL DEFAULT 0,
    updated_at    TEXT    NOT NULL
);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);
CREATE INDEX idx_sessions_user_project ON sessions(user_id, project_id);
CREATE INDEX idx_sessions_version ON sessions(version);

CREATE TABLE active_sessions (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE repos (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);

CREATE TABLE repo_notes (
    id         TEXT    PRIMARY KEY,
    repo_id    TEXT    NOT NULL REFERENCES repos(id),
    user_id    TEXT    NOT NULL REFERENCES users(id),
    content    TEXT    NOT NULL DEFAULT '',
    version    INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT    NOT NULL
);
CREATE INDEX idx_repo_notes_user ON repo_notes(user_id);

CREATE TABLE sync_state (
    resource    TEXT    PRIMARY KEY,
    watermark   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE write_queue (
    seq               INTEGER PRIMARY KEY AUTOINCREMENT,
    resource          TEXT    NOT NULL,
    row_id            TEXT    NOT NULL,
    payload           TEXT    NOT NULL,
    expected_version  INTEGER NOT NULL,
    enqueued_at       TEXT    NOT NULL,
    last_error        TEXT    NOT NULL DEFAULT ''
);
CREATE INDEX idx_write_queue_resource ON write_queue(resource);

-- +goose Down
DROP TABLE write_queue;
DROP TABLE sync_state;
DROP TABLE repo_notes;
DROP TABLE repos;
DROP TABLE active_sessions;
DROP TABLE sessions;
DROP TABLE projects;
DROP TABLE users;
