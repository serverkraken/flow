-- +goose Up
CREATE TABLE users (
    id           TEXT PRIMARY KEY,
    oidc_sub     TEXT NOT NULL UNIQUE,
    username     TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
