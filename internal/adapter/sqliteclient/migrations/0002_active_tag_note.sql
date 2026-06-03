-- +goose Up
ALTER TABLE active_sessions ADD COLUMN tag  TEXT NOT NULL DEFAULT '';
ALTER TABLE active_sessions ADD COLUMN note TEXT NOT NULL DEFAULT '';

-- +goose Down
-- SQLite < 3.35 lacks DROP COLUMN; rebuilding the table is required.
CREATE TABLE active_sessions_v1 (
    user_id            TEXT    NOT NULL REFERENCES users(id),
    project_id         TEXT    NOT NULL REFERENCES projects(id),
    started_at         TEXT    NOT NULL,
    started_on_device  TEXT    NOT NULL,
    version            INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, project_id)
);
INSERT INTO active_sessions_v1 (user_id, project_id, started_at, started_on_device, version)
SELECT user_id, project_id, started_at, started_on_device, version FROM active_sessions;
DROP TABLE active_sessions;
ALTER TABLE active_sessions_v1 RENAME TO active_sessions;
