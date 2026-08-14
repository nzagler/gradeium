-- name: GetSetupComplete :one
SELECT completed
FROM setup_state
WHERE singleton = true;

-- name: CompleteSetup :execrows
UPDATE setup_state
SET completed = true, completed_at = now(), updated_at = now()
WHERE singleton = true AND completed = false;
