-- +goose Up
ALTER TABLE user_settings
    ADD COLUMN theme text NOT NULL DEFAULT 'dark'
    CHECK (theme IN ('dark', 'light', 'system'));

-- +goose Down
ALTER TABLE user_settings DROP COLUMN theme;
