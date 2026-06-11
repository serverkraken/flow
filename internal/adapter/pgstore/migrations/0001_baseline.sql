-- +goose Up
CREATE TABLE users (
    id           uuid PRIMARY KEY,
    oidc_sub     text NOT NULL UNIQUE,
    email        text NOT NULL DEFAULT '',
    display_name text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id),
    name         text NOT NULL,
    slug         text NOT NULL,
    archived_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    version      bigint NOT NULL DEFAULT 1,
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, slug)
);

CREATE TABLE sessions (
    id         uuid PRIMARY KEY,        -- Client darf UUIDv5 liefern (Import-Idempotenz)
    user_id    uuid NOT NULL REFERENCES users(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    day        date NOT NULL,           -- Buchungstag in User-Zeitzone
    started_at timestamptz NOT NULL,
    stopped_at timestamptz NOT NULL,
    tag        text NOT NULL DEFAULT '',
    note       text NOT NULL DEFAULT '',
    version    bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_day ON sessions (user_id, day);

CREATE TABLE active_sessions (
    user_id           uuid NOT NULL REFERENCES users(id),
    project_id        uuid NOT NULL REFERENCES projects(id),
    started_at        timestamptz NOT NULL, -- Server-Zeit, nie Client-Zeit
    paused_at         timestamptz,          -- NULL = läuft
    pause_total_ns    bigint NOT NULL DEFAULT 0,
    started_on_device text NOT NULL DEFAULT '',
    tag               text NOT NULL DEFAULT '',
    note              text NOT NULL DEFAULT '',
    version           bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, project_id)
);

CREATE TABLE documents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id),
    path       text NOT NULL,
    body       text NOT NULL DEFAULT '',
    repo_key   text,
    version    bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    search     tsvector GENERATED ALWAYS AS (to_tsvector('simple', path || ' ' || body)) STORED,
    UNIQUE (user_id, path)
);
CREATE UNIQUE INDEX documents_repo_key ON documents (user_id, repo_key) WHERE repo_key IS NOT NULL;
CREATE INDEX documents_search ON documents USING gin (search);

CREATE TABLE day_offs (
    user_id   uuid NOT NULL REFERENCES users(id),
    day       date NOT NULL,
    kind      text NOT NULL,
    label     text NOT NULL DEFAULT '',
    target_ns bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, day)
);

CREATE TABLE user_settings (
    user_id uuid NOT NULL REFERENCES users(id),
    key     text NOT NULL,
    value   text NOT NULL,
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE IF EXISTS user_settings;
DROP TABLE IF EXISTS day_offs;
DROP TABLE IF EXISTS documents;
DROP TABLE IF EXISTS active_sessions;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS projects;
DROP TABLE IF EXISTS users;
