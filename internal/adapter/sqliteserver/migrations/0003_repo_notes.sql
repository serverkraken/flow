-- +goose Up
-- The initial M2 server schema already created repos + repo_notes tables,
-- but repos has no version column and neither table is indexed for the
-- (user_id, version) PullSince scan. Plan C / Task 5 fixes both.
ALTER TABLE repos ADD COLUMN version INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_repos_user_version ON repos(user_id, version);
CREATE INDEX idx_repo_notes_user_version ON repo_notes(user_id, version);
CREATE INDEX idx_repo_notes_repo ON repo_notes(repo_id);

-- +goose Down
DROP INDEX IF EXISTS idx_repo_notes_repo;
DROP INDEX IF EXISTS idx_repo_notes_user_version;
DROP INDEX IF EXISTS idx_repos_user_version;
-- SQLite < 3.35 lacks DROP COLUMN; rebuild the repos table for the down.
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
