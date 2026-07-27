-- +goose Up
CREATE TABLE tags (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id),
    slug       TEXT NOT NULL,
    display    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_id, slug)
);

CREATE TABLE taggings (
    tag_id        TEXT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    taggable_type TEXT NOT NULL CHECK (taggable_type IN ('document','node','work_session')),
    taggable_id   TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (tag_id, taggable_type, taggable_id)
);
CREATE INDEX taggings_taggable ON taggings (taggable_type, taggable_id);
CREATE INDEX taggings_tag      ON taggings (tag_id);

-- Backfill: document tags already live (normalized) in documents.tags[].
INSERT INTO tags (id, owner_id, slug, display, created_at)
SELECT DISTINCT 'tag-' || md5(d.owner_id || ':' || u.tag), d.owner_id, u.tag, u.tag, now()
FROM documents d
CROSS JOIN LATERAL unnest(d.tags) AS u(tag)
WHERE cardinality(d.tags) > 0
ON CONFLICT (owner_id, slug) DO NOTHING;

INSERT INTO taggings (tag_id, taggable_type, taggable_id, created_at)
SELECT t.id, 'document', d.id, now()
FROM documents d
CROSS JOIN LATERAL unnest(d.tags) AS u(tag)
JOIN tags t ON t.owner_id = d.owner_id AND t.slug = u.tag
ON CONFLICT (tag_id, taggable_type, taggable_id) DO NOTHING;

-- Backfill: work_sessions.tag is a single freetext value (normalize lower/trim).
INSERT INTO tags (id, owner_id, slug, display, created_at)
SELECT DISTINCT 'tag-' || md5(ws.owner_id || ':' || lower(btrim(ws.tag))), ws.owner_id, lower(btrim(ws.tag)), btrim(ws.tag), now()
FROM work_sessions ws
WHERE btrim(ws.tag) <> ''
ON CONFLICT (owner_id, slug) DO NOTHING;

INSERT INTO taggings (tag_id, taggable_type, taggable_id, created_at)
SELECT t.id, 'work_session', ws.id, now()
FROM work_sessions ws
JOIN tags t ON t.owner_id = ws.owner_id AND t.slug = lower(btrim(ws.tag))
WHERE btrim(ws.tag) <> ''
ON CONFLICT (tag_id, taggable_type, taggable_id) DO NOTHING;

-- +goose Down
DROP TABLE taggings;
DROP TABLE tags;
