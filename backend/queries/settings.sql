-- name: ListAppSettings :many
SELECT key, value
FROM app_settings
ORDER BY key;

-- name: UpsertAppSetting :exec
INSERT INTO app_settings (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value, updated_at = now();
