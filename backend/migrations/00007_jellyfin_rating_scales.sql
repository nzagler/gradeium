-- +goose Up
ALTER TABLE integration_test_status
    DROP CONSTRAINT integration_test_status_provider_check,
    ADD CONSTRAINT integration_test_status_provider_check
        CHECK (provider IN ('igdb', 'tmdb', 'tvdb', 'jellyfin'));

ALTER TABLE user_games
    DROP CONSTRAINT user_games_rating_check,
    ADD CONSTRAINT user_games_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 0 AND 100);

ALTER TABLE user_movies
    DROP CONSTRAINT user_movies_rating_check,
    ADD CONSTRAINT user_movies_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 0 AND 100);

ALTER TABLE user_tv_shows
    DROP CONSTRAINT user_tv_shows_rating_check,
    ADD CONSTRAINT user_tv_shows_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 0 AND 100);

ALTER TABLE user_settings
    ADD COLUMN rating_scale text NOT NULL DEFAULT '1_10'
        CHECK (rating_scale IN ('1_10', '0_5', 'minus5_plus5', '0_100'));

-- +goose Down
ALTER TABLE user_settings DROP COLUMN rating_scale;

ALTER TABLE user_tv_shows
    DROP CONSTRAINT user_tv_shows_rating_check,
    ADD CONSTRAINT user_tv_shows_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 10 AND 100);

ALTER TABLE user_movies
    DROP CONSTRAINT user_movies_rating_check,
    ADD CONSTRAINT user_movies_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 10 AND 100);

ALTER TABLE user_games
    DROP CONSTRAINT user_games_rating_check,
    ADD CONSTRAINT user_games_rating_check
        CHECK (rating IS NULL OR rating BETWEEN 10 AND 100);

ALTER TABLE integration_test_status
    DROP CONSTRAINT integration_test_status_provider_check,
    ADD CONSTRAINT integration_test_status_provider_check
        CHECK (provider IN ('igdb', 'tmdb', 'tvdb'));
