-- +goose Up
CREATE TABLE entities (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    type text NOT NULL CHECK (type IN ('game', 'movie', 'tv_show')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    external_subject text UNIQUE,
    display_name text,
    email text,
    is_admin boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE app_settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE secret_settings (
    key text PRIMARY KEY,
    algorithm_version smallint NOT NULL CHECK (algorithm_version > 0),
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) >= 16),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE setup_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    completed boolean NOT NULL DEFAULT false,
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((completed AND completed_at IS NOT NULL) OR (NOT completed AND completed_at IS NULL))
);

INSERT INTO setup_state (singleton, completed)
VALUES (true, false);

-- The fingerprint detects a missing or mismatched /config key without storing
-- the master key itself in PostgreSQL.
CREATE TABLE encryption_key_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    key_fingerprint bytea NOT NULL CHECK (octet_length(key_fingerprint) = 32),
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE encryption_key_state;
DROP TABLE setup_state;
DROP TABLE secret_settings;
DROP TABLE app_settings;
DROP TABLE users;
DROP TABLE entities;
