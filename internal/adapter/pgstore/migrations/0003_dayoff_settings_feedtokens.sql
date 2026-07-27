-- +goose Up
CREATE TABLE day_offs (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    day        DATE NOT NULL,
    kind       TEXT NOT NULL,
    label      TEXT NOT NULL DEFAULT '',
    target_min INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, day)
);
CREATE INDEX day_offs_owner_day ON day_offs (owner_id, day);

CREATE TABLE user_settings (
    user_id    TEXT PRIMARY KEY REFERENCES users(id),
    bundesland TEXT NOT NULL DEFAULT 'NW',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE feed_tokens (
    token      TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    kind       TEXT NOT NULL DEFAULT 'ics',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);
CREATE INDEX feed_tokens_user ON feed_tokens (user_id) WHERE revoked_at IS NULL;

-- +goose Down
DROP TABLE feed_tokens;
DROP TABLE user_settings;
DROP TABLE day_offs;
