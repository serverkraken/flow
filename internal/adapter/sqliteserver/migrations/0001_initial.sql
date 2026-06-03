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
CREATE INDEX idx_projects_user_version ON projects(user_id, version);

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
CREATE INDEX idx_sessions_user_version ON sessions(user_id, version);
CREATE INDEX idx_sessions_user_date ON sessions(user_id, date);

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

CREATE TABLE lamport (
    id        INTEGER PRIMARY KEY CHECK (id = 1),
    counter   INTEGER NOT NULL DEFAULT 0
);
INSERT INTO lamport(id, counter) VALUES (1, 0);

-- +goose Down
DROP TABLE lamport;
DROP TABLE repo_notes;
DROP TABLE repos;
DROP TABLE active_sessions;
DROP TABLE sessions;
DROP TABLE projects;
DROP TABLE users;
