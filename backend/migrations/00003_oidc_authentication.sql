-- +goose Up
ALTER TABLE users
    ADD COLUMN oidc_issuer text,
    ADD COLUMN oidc_subject text,
    ADD CONSTRAINT users_oidc_identity_pair CHECK (
        (oidc_issuer IS NULL AND oidc_subject IS NULL)
        OR (oidc_issuer IS NOT NULL AND oidc_subject IS NOT NULL)
    ),
    ADD CONSTRAINT users_oidc_issuer_length CHECK (
        oidc_issuer IS NULL OR length(oidc_issuer) BETWEEN 1 AND 2048
    ),
    ADD CONSTRAINT users_oidc_subject_length CHECK (
        oidc_subject IS NULL OR length(oidc_subject) BETWEEN 1 AND 512
    );

CREATE UNIQUE INDEX users_oidc_identity_unique
    ON users (oidc_issuer, oidc_subject)
    WHERE oidc_issuer IS NOT NULL AND oidc_subject IS NOT NULL;

CREATE TABLE authentication_state (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    configuration_revision bigint NOT NULL DEFAULT 0 CHECK (configuration_revision >= 0),
    validated_revision bigint CHECK (
        validated_revision IS NULL
        OR (validated_revision >= 0 AND validated_revision <= configuration_revision)
    ),
    validated_at timestamptz,
    activated boolean NOT NULL DEFAULT false,
    activated_at timestamptz,
    active_revision bigint,
    active_issuer_url text,
    active_client_id text,
    active_public_url text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (validated_revision IS NULL AND validated_at IS NULL)
        OR (validated_revision IS NOT NULL AND validated_at IS NOT NULL)
    ),
    CHECK (
        (NOT activated
            AND activated_at IS NULL
            AND active_revision IS NULL
            AND active_issuer_url IS NULL
            AND active_client_id IS NULL
            AND active_public_url IS NULL)
        OR
        (activated
            AND activated_at IS NOT NULL
            AND active_revision IS NOT NULL
            AND active_issuer_url IS NOT NULL
            AND active_client_id IS NOT NULL
            AND active_public_url IS NOT NULL)
    )
);

INSERT INTO authentication_state (singleton)
VALUES (true);

CREATE TABLE oidc_login_flows (
    state_hash bytea PRIMARY KEY CHECK (octet_length(state_hash) = 32),
    algorithm_version smallint NOT NULL CHECK (algorithm_version > 0),
    nonce bytea NOT NULL CHECK (octet_length(nonce) = 12),
    ciphertext bytea NOT NULL CHECK (octet_length(ciphertext) >= 16),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at)
);

CREATE INDEX oidc_login_flows_expires_at_idx ON oidc_login_flows (expires_at);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash) = 32),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX sessions_user_id_idx ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE sessions;
DROP TABLE oidc_login_flows;
DROP TABLE authentication_state;
DROP INDEX users_oidc_identity_unique;
ALTER TABLE users
    DROP CONSTRAINT users_oidc_subject_length,
    DROP CONSTRAINT users_oidc_issuer_length,
    DROP CONSTRAINT users_oidc_identity_pair,
    DROP COLUMN oidc_subject,
    DROP COLUMN oidc_issuer;
