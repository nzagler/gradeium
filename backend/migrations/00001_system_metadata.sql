-- +goose Up
CREATE TABLE system_metadata (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO system_metadata (key, value)
VALUES ('foundation_version', '1');

-- +goose Down
DROP TABLE system_metadata;
