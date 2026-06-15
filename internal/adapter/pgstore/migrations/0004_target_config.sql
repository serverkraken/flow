-- +goose Up
ALTER TABLE user_settings
    ADD COLUMN default_target_min INT NOT NULL DEFAULT 480,
    ADD COLUMN target_sun_min INT,
    ADD COLUMN target_mon_min INT,
    ADD COLUMN target_tue_min INT,
    ADD COLUMN target_wed_min INT,
    ADD COLUMN target_thu_min INT,
    ADD COLUMN target_fri_min INT,
    ADD COLUMN target_sat_min INT;

-- +goose Down
ALTER TABLE user_settings
    DROP COLUMN default_target_min,
    DROP COLUMN target_sun_min,
    DROP COLUMN target_mon_min,
    DROP COLUMN target_tue_min,
    DROP COLUMN target_wed_min,
    DROP COLUMN target_thu_min,
    DROP COLUMN target_fri_min,
    DROP COLUMN target_sat_min;
