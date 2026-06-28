-- +goose Up
-- Slugs become unique per sibling set, not globally. The same name may now repeat
-- across the tree (e.g. a repo named like its parent vorhaben). The old global
-- UNIQUE(owner_id, slug) — still carrying its pre-rename name projects_owner_id_slug_key
-- — is replaced by two partial unique indexes:
--   * roots (parent_id IS NULL, i.e. engagements) stay globally unique per owner;
--   * children are unique only among siblings (owner_id, parent_id, slug).
-- Two indexes are used instead of one UNIQUE(owner_id, parent_id, slug) because SQL
-- treats NULL parents as distinct, which would wrongly allow two roots to share a slug.
ALTER TABLE nodes DROP CONSTRAINT IF EXISTS projects_owner_id_slug_key;

CREATE UNIQUE INDEX nodes_root_slug ON nodes (owner_id, slug) WHERE parent_id IS NULL;
CREATE UNIQUE INDEX nodes_child_slug ON nodes (owner_id, parent_id, slug) WHERE parent_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS nodes_child_slug;
DROP INDEX IF EXISTS nodes_root_slug;
-- Re-adding the global constraint fails if duplicate slugs now exist (expected: you
-- cannot downgrade once the per-sibling capability has been used).
ALTER TABLE nodes ADD CONSTRAINT projects_owner_id_slug_key UNIQUE (owner_id, slug);
