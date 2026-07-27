-- +goose Up
ALTER TABLE artifacts ALTER COLUMN node_id DROP NOT NULL;
-- Der Bestands-unique(owner_id,node_id,slug) greift bei NULL-Zeilen NICHT
-- (Postgres behandelt NULL als distinkt) → Partial-Unique-Index für frei.
CREATE UNIQUE INDEX artifacts_owner_free_slug ON artifacts (owner_id, slug) WHERE node_id IS NULL;

-- +goose Down
-- Achtung: DROP INDEX zuerst, dann NOT NULL zurück. SET NOT NULL schlägt fehl,
-- wenn freie (NULL-)Zeilen existieren — Down ist ein Entwicklungs-Rollback,
-- kein PROD-Pfad; der Betreiber muss freie Artefakte vorher entfernen.
DROP INDEX artifacts_owner_free_slug;
ALTER TABLE artifacts ALTER COLUMN node_id SET NOT NULL;
