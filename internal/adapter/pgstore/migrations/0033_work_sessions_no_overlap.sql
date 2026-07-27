-- +goose Up
-- Equality on owner_id plus range-overlap on timestamptz requires btree_gist.
-- Adding the constraint validates existing rows atomically: a pre-existing
-- overlap aborts this migration without modifying session data.
CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE work_sessions
    ADD CONSTRAINT work_sessions_no_overlap
    EXCLUDE USING gist (
        owner_id WITH =,
        tstzrange(start_at, COALESCE(stop_at, 'infinity'::timestamptz), '[)') WITH &&
    );

-- +goose Down
ALTER TABLE work_sessions DROP CONSTRAINT IF EXISTS work_sessions_no_overlap;
