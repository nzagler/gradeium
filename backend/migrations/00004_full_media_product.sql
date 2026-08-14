-- +goose Up
CREATE TYPE media_status AS ENUM (
    'backlog',
    'in_progress',
    'on_hold',
    'abandoned',
    'completed'
);

CREATE TABLE integration_test_status (
    provider text PRIMARY KEY CHECK (provider IN ('igdb', 'tmdb', 'tvdb')),
    status text NOT NULL CHECK (status IN ('connected', 'error')),
    message text NOT NULL CHECK (length(message) BETWEEN 1 AND 240),
    tested_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_settings (
    user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    default_library_sort text NOT NULL DEFAULT 'rating_desc' CHECK (
        default_library_sort IN (
            'rating_desc', 'rating_asc', 'community_desc',
            'title_asc', 'title_desc', 'release_desc', 'release_asc',
            'added_desc', 'added_asc'
        )
    ),
    preferred_view text NOT NULL DEFAULT 'grid' CHECK (preferred_view IN ('grid', 'list')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE games (
    entity_id uuid PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    igdb_id bigint NOT NULL UNIQUE CHECK (igdb_id > 0),
    english_title text NOT NULL CHECK (length(english_title) BETWEEN 1 AND 500),
    original_title text,
    summary text,
    first_release_date date,
    release_year integer CHECK (release_year IS NULL OR release_year BETWEEN 1800 AND 3000),
    game_type text NOT NULL,
    developer text,
    publisher text,
    genres text[] NOT NULL DEFAULT '{}',
    game_modes text[] NOT NULL DEFAULT '{}',
    platforms text[] NOT NULL DEFAULT '{}',
    franchise text,
    community_rating smallint CHECK (community_rating IS NULL OR community_rating BETWEEN 10 AND 100),
    community_rating_count integer CHECK (community_rating_count IS NULL OR community_rating_count >= 0),
    screenshots jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(screenshots) = 'array'),
    external_links jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(external_links) = 'array'),
    metadata_refreshed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX games_title_idx ON games (lower(english_title));
CREATE INDEX games_release_year_idx ON games (release_year);

CREATE TABLE game_additional_content (
    game_id uuid NOT NULL REFERENCES games(entity_id) ON DELETE CASCADE,
    igdb_id bigint NOT NULL CHECK (igdb_id > 0),
    title text NOT NULL,
    content_type text NOT NULL,
    release_year integer,
    cover_url text,
    PRIMARY KEY (game_id, igdb_id)
);

CREATE TABLE game_related_releases (
    game_id uuid NOT NULL REFERENCES games(entity_id) ON DELETE CASCADE,
    igdb_id bigint NOT NULL CHECK (igdb_id > 0),
    title text NOT NULL,
    relationship text NOT NULL CHECK (relationship IN ('original', 'remake', 'remaster', 'franchise')),
    release_year integer,
    cover_url text,
    PRIMARY KEY (game_id, igdb_id, relationship)
);

CREATE TABLE movies (
    entity_id uuid PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    tmdb_id bigint NOT NULL UNIQUE CHECK (tmdb_id > 0),
    english_title text NOT NULL CHECK (length(english_title) BETWEEN 1 AND 500),
    original_title text,
    overview text,
    release_date date,
    release_year integer CHECK (release_year IS NULL OR release_year BETWEEN 1800 AND 3000),
    runtime_minutes integer CHECK (runtime_minutes IS NULL OR runtime_minutes BETWEEN 1 AND 10000),
    director text,
    genres text[] NOT NULL DEFAULT '{}',
    production_companies text[] NOT NULL DEFAULT '{}',
    cast_members jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(cast_members) = 'array'),
    key_crew jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(key_crew) = 'array'),
    trailer_key text,
    imdb_id text,
    homepage text,
    collection_tmdb_id bigint,
    collection_name text,
    community_rating smallint CHECK (community_rating IS NULL OR community_rating BETWEEN 10 AND 100),
    community_rating_count integer CHECK (community_rating_count IS NULL OR community_rating_count >= 0),
    metadata_refreshed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((collection_tmdb_id IS NULL) = (collection_name IS NULL))
);

CREATE INDEX movies_title_idx ON movies (lower(english_title));
CREATE INDEX movies_release_year_idx ON movies (release_year);

CREATE TABLE movie_collection_members (
    movie_id uuid NOT NULL REFERENCES movies(entity_id) ON DELETE CASCADE,
    tmdb_id bigint NOT NULL CHECK (tmdb_id > 0),
    title text NOT NULL,
    release_date date,
    poster_url text,
    PRIMARY KEY (movie_id, tmdb_id)
);

CREATE TABLE tv_shows (
    entity_id uuid PRIMARY KEY REFERENCES entities(id) ON DELETE CASCADE,
    tvdb_id bigint NOT NULL UNIQUE CHECK (tvdb_id > 0),
    verified_tmdb_id bigint UNIQUE CHECK (verified_tmdb_id IS NULL OR verified_tmdb_id > 0),
    english_title text NOT NULL CHECK (length(english_title) BETWEEN 1 AND 500),
    original_title text,
    overview text,
    first_aired date,
    release_year integer CHECK (release_year IS NULL OR release_year BETWEEN 1800 AND 3000),
    provider_status text,
    network_name text,
    genres text[] NOT NULL DEFAULT '{}',
    cast_members jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(cast_members) = 'array'),
    key_people jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(key_people) = 'array'),
    community_rating smallint CHECK (community_rating IS NULL OR community_rating BETWEEN 10 AND 100),
    community_rating_count integer CHECK (community_rating_count IS NULL OR community_rating_count >= 0),
    tmdb_mapping_verified_at timestamptz,
    metadata_refreshed_at timestamptz NOT NULL DEFAULT now(),
    community_rating_refreshed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((verified_tmdb_id IS NULL) = (tmdb_mapping_verified_at IS NULL))
);

CREATE INDEX tv_shows_title_idx ON tv_shows (lower(english_title));
CREATE INDEX tv_shows_release_year_idx ON tv_shows (release_year);

CREATE TABLE tv_seasons (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tv_show_id uuid NOT NULL REFERENCES tv_shows(entity_id) ON DELETE CASCADE,
    tvdb_season_id bigint NOT NULL UNIQUE CHECK (tvdb_season_id > 0),
    season_number integer NOT NULL CHECK (season_number >= 0),
    name text,
    is_specials boolean NOT NULL,
    air_date date,
    poster_url text,
    available boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tv_show_id, season_number),
    UNIQUE (id, tv_show_id),
    CHECK (is_specials = (season_number = 0))
);

CREATE TABLE tv_episodes (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    tv_show_id uuid NOT NULL REFERENCES tv_shows(entity_id) ON DELETE CASCADE,
    season_id uuid NOT NULL,
    tvdb_episode_id bigint NOT NULL UNIQUE CHECK (tvdb_episode_id > 0),
    season_number integer NOT NULL CHECK (season_number >= 0),
    episode_number integer NOT NULL CHECK (episode_number >= 0),
    sort_order integer NOT NULL CHECK (sort_order >= 0),
    english_title text NOT NULL CHECK (length(english_title) BETWEEN 1 AND 500),
    overview text,
    air_date date,
    runtime_minutes integer CHECK (runtime_minutes IS NULL OR runtime_minutes BETWEEN 1 AND 10000),
    still_url text,
    is_special boolean NOT NULL,
    available boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (season_id, tv_show_id) REFERENCES tv_seasons(id, tv_show_id) ON DELETE CASCADE,
    UNIQUE (tv_show_id, season_number, episode_number),
    UNIQUE (id, tv_show_id),
    CHECK (is_special = (season_number = 0))
);

CREATE INDEX tv_episodes_show_order_idx ON tv_episodes (tv_show_id, is_special, sort_order);

CREATE TABLE media_artwork (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    entity_id uuid NOT NULL REFERENCES entities(id) ON DELETE CASCADE,
    provider text NOT NULL CHECK (provider IN ('igdb', 'tmdb', 'tvdb')),
    provider_image_id text NOT NULL CHECK (length(provider_image_id) BETWEEN 1 AND 500),
    kind text NOT NULL CHECK (kind IN ('poster', 'cover', 'backdrop', 'logo')),
    language text,
    image_url text NOT NULL CHECK (image_url ~ '^https://'),
    thumbnail_url text NOT NULL CHECK (thumbnail_url ~ '^https://'),
    width integer CHECK (width IS NULL OR width > 0),
    height integer CHECK (height IS NULL OR height > 0),
    preferred boolean NOT NULL DEFAULT false,
    available boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (entity_id, provider_image_id),
    UNIQUE (entity_id, kind, provider_image_id)
);

CREATE INDEX media_artwork_entity_kind_idx ON media_artwork (entity_id, kind, preferred DESC, sort_order);

CREATE TABLE user_games (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    game_id uuid NOT NULL REFERENCES games(entity_id) ON DELETE CASCADE,
    status media_status NOT NULL,
    rating smallint CHECK (rating IS NULL OR rating BETWEEN 10 AND 100),
    rating_reason text CHECK (rating_reason IS NULL OR length(rating_reason) BETWEEN 1 AND 4000),
    selected_cover_provider_image_id text,
    selected_backdrop_provider_image_id text,
    selected_logo_provider_image_id text,
    date_added timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, game_id),
    FOREIGN KEY (game_id, selected_cover_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (game_id, selected_backdrop_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (game_id, selected_logo_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    CHECK ((rating IS NOT NULL) OR rating_reason IS NULL),
    CHECK (status <> 'backlog' OR (rating IS NULL AND rating_reason IS NULL))
);

CREATE TABLE user_movies (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    movie_id uuid NOT NULL REFERENCES movies(entity_id) ON DELETE CASCADE,
    status media_status NOT NULL,
    rating smallint CHECK (rating IS NULL OR rating BETWEEN 10 AND 100),
    rating_reason text CHECK (rating_reason IS NULL OR length(rating_reason) BETWEEN 1 AND 4000),
    selected_poster_provider_image_id text,
    selected_backdrop_provider_image_id text,
    selected_logo_provider_image_id text,
    date_added timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, movie_id),
    FOREIGN KEY (movie_id, selected_poster_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (movie_id, selected_backdrop_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (movie_id, selected_logo_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    CHECK ((rating IS NOT NULL) OR rating_reason IS NULL),
    CHECK (status <> 'backlog' OR (rating IS NULL AND rating_reason IS NULL))
);

CREATE TABLE user_tv_shows (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tv_show_id uuid NOT NULL REFERENCES tv_shows(entity_id) ON DELETE CASCADE,
    status media_status NOT NULL,
    rating smallint CHECK (rating IS NULL OR rating BETWEEN 10 AND 100),
    rating_reason text CHECK (rating_reason IS NULL OR length(rating_reason) BETWEEN 1 AND 4000),
    selected_poster_provider_image_id text,
    selected_backdrop_provider_image_id text,
    selected_logo_provider_image_id text,
    date_added timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, tv_show_id),
    FOREIGN KEY (tv_show_id, selected_poster_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (tv_show_id, selected_backdrop_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    FOREIGN KEY (tv_show_id, selected_logo_provider_image_id) REFERENCES media_artwork(entity_id, provider_image_id),
    CHECK ((rating IS NOT NULL) OR rating_reason IS NULL),
    CHECK (status <> 'backlog' OR (rating IS NULL AND rating_reason IS NULL))
);

CREATE INDEX user_games_library_idx ON user_games (user_id, status, rating DESC NULLS LAST, date_added DESC);
CREATE INDEX user_movies_library_idx ON user_movies (user_id, status, rating DESC NULLS LAST, date_added DESC);
CREATE INDEX user_tv_shows_library_idx ON user_tv_shows (user_id, status, rating DESC NULLS LAST, date_added DESC);

CREATE TABLE user_episode_progress (
    user_id uuid NOT NULL,
    tv_show_id uuid NOT NULL,
    episode_id uuid NOT NULL,
    watched_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, episode_id),
    FOREIGN KEY (user_id, tv_show_id) REFERENCES user_tv_shows(user_id, tv_show_id) ON DELETE CASCADE,
    FOREIGN KEY (episode_id, tv_show_id) REFERENCES tv_episodes(id, tv_show_id) ON DELETE CASCADE
);

CREATE INDEX user_episode_progress_show_idx ON user_episode_progress (user_id, tv_show_id);

-- +goose Down
DROP TABLE user_episode_progress;
DROP TABLE user_tv_shows;
DROP TABLE user_movies;
DROP TABLE user_games;
DROP TABLE media_artwork;
DROP TABLE tv_episodes;
DROP TABLE tv_seasons;
DROP TABLE tv_shows;
DROP TABLE movie_collection_members;
DROP TABLE movies;
DROP TABLE game_related_releases;
DROP TABLE game_additional_content;
DROP TABLE games;
DROP TABLE user_settings;
DROP TABLE integration_test_status;
DROP TYPE media_status;
