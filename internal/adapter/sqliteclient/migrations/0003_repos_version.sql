-- +goose Up
ALTER TABLE repos ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_repos_user_version ON repos(user_id, version);

-- +goose Down
-- SQLite < 3.35 lacks DROP COLUMN; rebuilding the table is required.
DROP INDEX IF EXISTS idx_repos_user_version;
CREATE TABLE repos_v1 (
    id             TEXT    PRIMARY KEY,
    user_id        TEXT    NOT NULL REFERENCES users(id),
    canonical_key  TEXT    NOT NULL,
    display_name   TEXT    NOT NULL DEFAULT '',
    created_at     TEXT    NOT NULL,
    UNIQUE(user_id, canonical_key)
);
INSERT INTO repos_v1 (id, user_id, canonical_key, display_name, created_at)
SELECT id, user_id, canonical_key, display_name, created_at FROM repos;
DROP TABLE repos;
ALTER TABLE repos_v1 RENAME TO repos;
