-- +goose Up
ALTER TABLE nodes ADD COLUMN weekly_target_min INTEGER;

-- +goose Down
ALTER TABLE nodes DROP COLUMN weekly_target_min;
