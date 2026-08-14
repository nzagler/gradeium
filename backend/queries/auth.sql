-- name: GetAuthenticationState :one
SELECT configuration_revision,
       validated_revision,
       validated_at,
       activated,
       activated_at,
       active_revision,
       active_issuer_url,
       active_client_id,
       active_public_url
FROM authentication_state
WHERE singleton = true;

-- name: GetAuthenticationStateForUpdate :one
SELECT configuration_revision,
       validated_revision,
       activated,
       active_revision,
       active_issuer_url,
       active_client_id,
       active_public_url
FROM authentication_state
WHERE singleton = true
FOR UPDATE;

-- name: IncrementAuthenticationConfigurationRevision :one
UPDATE authentication_state
SET configuration_revision = configuration_revision + 1,
    validated_revision = NULL,
    validated_at = NULL,
    updated_at = now()
WHERE singleton = true
RETURNING configuration_revision;

-- name: MarkAuthenticationConfigurationValidated :execrows
UPDATE authentication_state
SET validated_revision = $1,
    validated_at = now(),
    updated_at = now()
WHERE singleton = true
  AND configuration_revision = $1;

-- name: ActivateAuthentication :execrows
UPDATE authentication_state
SET activated = true,
    activated_at = now(),
    active_revision = $1,
    active_issuer_url = $2,
    active_client_id = $3,
    active_public_url = $4,
    updated_at = now()
WHERE singleton = true
  AND activated = false
  AND configuration_revision = $1
  AND validated_revision = $1;

-- name: ApplyActiveAuthenticationConfiguration :execrows
UPDATE authentication_state
SET active_revision = $1,
    active_issuer_url = $2,
    active_client_id = $3,
    active_public_url = $4,
    updated_at = now()
WHERE singleton = true
  AND activated = true
  AND configuration_revision = $1
  AND validated_revision = $1;

-- name: GetAppSetting :one
SELECT value
FROM app_settings
WHERE key = $1;

-- name: InsertOIDCLoginFlow :exec
INSERT INTO oidc_login_flows (
    state_hash, algorithm_version, nonce, ciphertext, expires_at
) VALUES ($1, $2, $3, $4, $5);

-- name: ConsumeOIDCLoginFlow :one
DELETE FROM oidc_login_flows
WHERE state_hash = $1
RETURNING algorithm_version, nonce, ciphertext, expires_at;

-- name: DeleteExpiredOIDCLoginFlows :execrows
WITH expired AS (
    SELECT state_hash
    FROM oidc_login_flows
    WHERE expires_at <= now()
    ORDER BY expires_at
    LIMIT $1
)
DELETE FROM oidc_login_flows
WHERE state_hash IN (SELECT state_hash FROM expired);

-- name: GetUserByOIDCIdentity :one
SELECT id, oidc_issuer, oidc_subject, display_name, email, is_admin, created_at, updated_at
FROM users
WHERE oidc_issuer = $1 AND oidc_subject = $2;

-- name: AnyAdminExists :one
SELECT EXISTS (SELECT 1 FROM users WHERE is_admin = true);

-- name: CreateOIDCUser :one
INSERT INTO users (oidc_issuer, oidc_subject, display_name, email, is_admin)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, oidc_issuer, oidc_subject, display_name, email, is_admin, created_at, updated_at;

-- name: UpdateOIDCUser :one
UPDATE users
SET display_name = $2,
    email = $3,
    updated_at = now()
WHERE id = $1
RETURNING id, oidc_issuer, oidc_subject, display_name, email, is_admin, created_at, updated_at;

-- name: PromoteUserToAdmin :one
UPDATE users
SET is_admin = true,
    updated_at = now()
WHERE id = $1 AND is_admin = false
RETURNING id, oidc_issuer, oidc_subject, display_name, email, is_admin, created_at, updated_at;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, expires_at)
VALUES ($1, $2, $3)
RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at, updated_at;

-- name: GetSessionByTokenHash :one
SELECT s.id,
       s.user_id,
       s.expires_at,
       s.revoked_at,
       u.oidc_issuer,
       u.oidc_subject,
       u.display_name,
       u.email,
       u.is_admin,
       a.active_public_url
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN authentication_state a ON a.singleton = true
WHERE s.token_hash = $1;

-- name: RevokeSessionByTokenHash :execrows
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeSessionByID :execrows
UPDATE sessions
SET revoked_at = COALESCE(revoked_at, now()),
    updated_at = now()
WHERE id = $1 AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :execrows
WITH expired AS (
    SELECT id
    FROM sessions
    WHERE expires_at <= now()
       OR (revoked_at IS NOT NULL AND revoked_at <= now() - interval '1 day')
    ORDER BY expires_at
    LIMIT $1
)
DELETE FROM sessions
WHERE id IN (SELECT id FROM expired);
