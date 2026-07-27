-- +goose Up
ALTER TABLE projects
    ADD COLUMN rate_amount   BIGINT,
    ADD COLUMN rate_currency TEXT;

-- +goose Down
ALTER TABLE projects
    DROP COLUMN rate_amount,
    DROP COLUMN rate_currency;
