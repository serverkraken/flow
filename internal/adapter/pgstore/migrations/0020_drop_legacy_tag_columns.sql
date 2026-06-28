-- +goose Up
-- The tags now live entirely in taggings (backfilled by 0019); no code selects
-- these columns after the B2 read-cutover. Drop them.
DROP INDEX IF EXISTS documents_tags_gin;
ALTER TABLE documents DROP COLUMN tags;
ALTER TABLE work_sessions DROP COLUMN tag;

-- +goose Down
ALTER TABLE work_sessions ADD COLUMN tag TEXT NOT NULL DEFAULT '';
ALTER TABLE documents ADD COLUMN tags TEXT[] NOT NULL DEFAULT '{}';
CREATE INDEX documents_tags_gin ON documents USING GIN (tags);
-- Re-project tags from taggings (best-effort; display casing not restored).
UPDATE documents d SET tags = sub.arr
FROM (SELECT tg.taggable_id AS id, array_agg(t.slug ORDER BY t.slug) AS arr
      FROM taggings tg JOIN tags t ON t.id = tg.tag_id
      WHERE tg.taggable_type='document' GROUP BY tg.taggable_id) sub
WHERE d.id = sub.id;
