-- +goose Up
ALTER TABLE node_logos ADD COLUMN width  integer NOT NULL DEFAULT 0;
ALTER TABLE node_logos ADD COLUMN height integer NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE node_logos DROP COLUMN height;
ALTER TABLE node_logos DROP COLUMN width;
