-- +goose Up
ALTER TABLE nodes ALTER COLUMN counts_toward_target DROP NOT NULL;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET DEFAULT NULL;
-- Existing rows are all the old default `true`; treat them as "inherit" so the
-- new inheritance model works out of the box. Explicit `false` (Privat) is kept.
UPDATE nodes SET counts_toward_target = NULL WHERE counts_toward_target = true;

-- +goose Down
UPDATE nodes SET counts_toward_target = true WHERE counts_toward_target IS NULL;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET DEFAULT true;
ALTER TABLE nodes ALTER COLUMN counts_toward_target SET NOT NULL;
