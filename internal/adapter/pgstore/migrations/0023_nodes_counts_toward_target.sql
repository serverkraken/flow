-- +goose Up
ALTER TABLE nodes ADD COLUMN counts_toward_target BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE nodes DROP COLUMN counts_toward_target;
