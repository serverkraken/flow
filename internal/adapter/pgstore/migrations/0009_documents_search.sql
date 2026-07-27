-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

ALTER TABLE documents ADD COLUMN search tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('simple', coalesce(title, '')), 'A') ||
    setweight(to_tsvector('simple', coalesce(body, '')),  'B')
) STORED;
CREATE INDEX documents_search_gin ON documents USING GIN (search);

CREATE INDEX documents_trgm_title ON documents USING GIN (title gin_trgm_ops);
CREATE INDEX documents_trgm_body  ON documents USING GIN (body  gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS documents_trgm_body;
DROP INDEX IF EXISTS documents_trgm_title;
DROP INDEX IF EXISTS documents_search_gin;
ALTER TABLE documents DROP COLUMN IF EXISTS search;
-- pg_trgm extension intentionally left installed (harmless, shared).
