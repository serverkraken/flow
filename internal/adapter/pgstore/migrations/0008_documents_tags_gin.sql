-- Speeds up the `tags @> ARRAY[...]` containment filter used by tag filtering.
CREATE INDEX documents_tags_gin ON documents USING GIN (tags);
