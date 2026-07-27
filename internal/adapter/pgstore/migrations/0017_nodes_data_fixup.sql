-- +goose Up

-- 1. Ensure the two default engagements exist for every owner that has >=1 node.
INSERT INTO nodes (id, owner_id, parent_id, kind, name, slug, status, extra, created_at, updated_at)
SELECT 'eng-privat-' || o.owner_id, o.owner_id, NULL, 'engagement', 'Privat', 'privat', 'active', '{}', now(), now()
FROM (SELECT DISTINCT owner_id FROM nodes) o
WHERE NOT EXISTS (SELECT 1 FROM nodes n WHERE n.owner_id = o.owner_id AND n.slug = 'privat');

INSERT INTO nodes (id, owner_id, parent_id, kind, name, slug, status, extra, created_at, updated_at)
SELECT 'eng-rtl-extern-' || o.owner_id, o.owner_id, NULL, 'engagement', 'RTL Extern', 'rtl-extern', 'active', '{}', now(), now()
FROM (SELECT DISTINCT owner_id FROM nodes) o
WHERE NOT EXISTS (SELECT 1 FROM nodes n WHERE n.owner_id = o.owner_id AND n.slug = 'rtl-extern');

-- 2. Parent the legacy repos under an engagement (slug ~ gitlab → RTL Extern, else Privat).
--    Idempotent: only rows still at the root (parent_id IS NULL) are touched.
UPDATE nodes r
SET parent_id = e.id
FROM nodes e
WHERE r.kind = 'repo'
  AND r.parent_id IS NULL
  AND r.slug NOT IN ('privat','rtl-extern')
  AND e.owner_id = r.owner_id
  AND e.kind = 'engagement'
  AND e.slug = CASE WHEN r.slug ILIKE '%gitlab%' THEN 'rtl-extern' ELSE 'privat' END;

-- 3. Audit + clear rate on repos (rate belongs to the engagement now).
UPDATE nodes
SET extra = jsonb_set(extra, '{legacy_rate}',
        jsonb_build_object('amount', rate_amount, 'currency', rate_currency)),
    rate_amount = NULL,
    rate_currency = NULL
WHERE kind = 'repo' AND rate_amount IS NOT NULL;

-- 4. Re-scope documents by category. daily → RTL engagement; free → global (NULL).
--    project/agent keep their node_id (now a repo); instruction/memory stay NULL.
UPDATE documents d
SET node_id = e.id
FROM nodes e
WHERE d.type = 'daily'
  AND e.owner_id = d.owner_id
  AND e.kind = 'engagement'
  AND e.slug = 'rtl-extern';

UPDATE documents SET node_id = NULL WHERE type = 'free';

-- 5. Re-point work sessions from the repo to its engagement parent (booking = engagement).
--    Idempotent: once node_id points at the engagement the join no longer matches a repo.
UPDATE work_sessions ws
SET node_id = r.parent_id
FROM nodes r
WHERE ws.node_id = r.id
  AND r.kind = 'repo'
  AND r.parent_id IS NOT NULL;

-- 6. Drop the temporary kind default and lock in the static invariants.
ALTER TABLE nodes ALTER COLUMN kind DROP DEFAULT;

ALTER TABLE nodes ADD CONSTRAINT nodes_kind_enum
    CHECK (kind IN ('engagement','vorhaben','repo','branch'));
ALTER TABLE nodes ADD CONSTRAINT nodes_root_is_engagement
    CHECK (parent_id IS NOT NULL OR kind = 'engagement');
ALTER TABLE nodes ADD CONSTRAINT nodes_rate_only_engagement
    CHECK (rate_amount IS NULL OR kind = 'engagement');
ALTER TABLE nodes ADD CONSTRAINT nodes_origin_only_repo
    CHECK (origin_slug IS NULL OR kind = 'repo');

-- +goose Down
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_origin_only_repo;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_rate_only_engagement;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_root_is_engagement;
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS nodes_kind_enum;
ALTER TABLE nodes ALTER COLUMN kind SET DEFAULT 'repo';
-- Data transformations are not reversed (audit lives in extra.legacy_rate).
