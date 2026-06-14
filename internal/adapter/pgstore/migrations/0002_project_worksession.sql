-- +goose Up
CREATE TABLE projects (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    color      TEXT NOT NULL DEFAULT '',
    glyph      TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug)
);

CREATE TABLE work_sessions (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    project_id TEXT REFERENCES projects(id),
    tag        TEXT NOT NULL DEFAULT '',
    note       TEXT NOT NULL DEFAULT '',
    start_at   TIMESTAMPTZ NOT NULL,
    stop_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- at most one running (unstopped) session per owner
CREATE UNIQUE INDEX one_running_session_per_user
    ON work_sessions (owner_id) WHERE stop_at IS NULL;
CREATE INDEX work_sessions_owner_start
    ON work_sessions (owner_id, start_at DESC);

-- +goose Down
DROP TABLE work_sessions;
DROP TABLE projects;
