-- name: GetIntegrationTestStatus :one
SELECT provider, status, message, tested_at, updated_at
FROM integration_test_status
WHERE provider = $1;

-- name: ListIntegrationTestStatuses :many
SELECT provider, status, message, tested_at, updated_at
FROM integration_test_status
ORDER BY provider;

-- name: UpsertIntegrationTestStatus :one
INSERT INTO integration_test_status (provider, status, message, tested_at, updated_at)
VALUES ($1, $2, $3, now(), now())
ON CONFLICT (provider) DO UPDATE SET
    status = EXCLUDED.status,
    message = EXCLUDED.message,
    tested_at = now(),
    updated_at = now()
RETURNING provider, status, message, tested_at, updated_at;

-- name: DeleteIntegrationTestStatus :exec
DELETE FROM integration_test_status
WHERE provider = $1;
