-- +goose Up
-- Plan F · Task 8: exponential-backoff retry policy for the httpsync worker.
--   attempt        — number of failed push attempts for this entry. Used as
--                    the exponent in Backoff.For so each retry waits longer
--                    than the last.
--   next_retry_at  — RFC3339 timestamp the entry becomes eligible for retry.
--                    Peek filters out rows with next_retry_at > now so the
--                    worker drains only what is due.
ALTER TABLE write_queue ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0;
ALTER TABLE write_queue ADD COLUMN next_retry_at TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_write_queue_next_retry_at ON write_queue(next_retry_at);

-- +goose Down
-- SQLite < 3.35 lacks DROP COLUMN; rebuild the table to mirror the 0001
-- schema. modernc.org/sqlite v1.49.x ships SQLite 3.45+ which does support
-- DROP COLUMN, but rebuilding is portable and matches 0003_repos_version's
-- pattern.
DROP INDEX IF EXISTS idx_write_queue_next_retry_at;
CREATE TABLE write_queue_v1 (
    seq               INTEGER PRIMARY KEY AUTOINCREMENT,
    resource          TEXT    NOT NULL,
    row_id            TEXT    NOT NULL,
    payload           TEXT    NOT NULL,
    expected_version  INTEGER NOT NULL,
    enqueued_at       TEXT    NOT NULL,
    last_error        TEXT    NOT NULL DEFAULT ''
);
INSERT INTO write_queue_v1 (seq, resource, row_id, payload, expected_version, enqueued_at, last_error)
SELECT seq, resource, row_id, payload, expected_version, enqueued_at, last_error FROM write_queue;
DROP TABLE write_queue;
ALTER TABLE write_queue_v1 RENAME TO write_queue;
CREATE INDEX idx_write_queue_resource ON write_queue(resource);
