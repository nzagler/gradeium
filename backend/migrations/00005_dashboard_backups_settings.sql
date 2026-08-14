-- +goose Up
CREATE TABLE backup_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    enabled boolean NOT NULL DEFAULT true,
    interval_days integer NOT NULL DEFAULT 3 CHECK (interval_days BETWEEN 1 AND 365),
    retention_count integer NOT NULL DEFAULT 30 CHECK (retention_count BETWEEN 1 AND 365),
    schedule_anchor_at timestamptz NOT NULL DEFAULT now(),
    last_attempt_at timestamptz,
    last_successful_automatic_at timestamptz,
    last_error text CHECK (last_error IS NULL OR length(last_error) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO backup_settings (singleton)
VALUES (true);

CREATE TABLE backups (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    filename text NOT NULL UNIQUE CHECK (
        filename ~ '^gradeium-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-(manual|automatic|pre-restore)\.json\.gz$'
    ),
    kind text NOT NULL CHECK (kind IN ('manual', 'automatic', 'pre_restore')),
    created_at timestamptz NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    sha256 text NOT NULL CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    format_version integer NOT NULL CHECK (format_version > 0),
    application_version text NOT NULL CHECK (length(application_version) BETWEEN 1 AND 100),
    valid boolean NOT NULL DEFAULT true,
    recorded_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX backups_kind_created_at_idx ON backups (kind, created_at DESC);

-- +goose Down
DROP TABLE backups;
DROP TABLE backup_settings;
