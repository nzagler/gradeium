-- name: GetSecretSetting :one
SELECT key, algorithm_version, nonce, ciphertext
FROM secret_settings
WHERE key = $1;

-- name: ListSecretSettings :many
SELECT key, algorithm_version, nonce, ciphertext
FROM secret_settings
ORDER BY key;

-- name: UpsertSecretSetting :exec
INSERT INTO secret_settings (key, algorithm_version, nonce, ciphertext)
VALUES ($1, $2, $3, $4)
ON CONFLICT (key) DO UPDATE
SET algorithm_version = EXCLUDED.algorithm_version,
    nonce = EXCLUDED.nonce,
    ciphertext = EXCLUDED.ciphertext,
    updated_at = now();

-- name: DeleteSecretSetting :execrows
DELETE FROM secret_settings
WHERE key = $1;

-- name: CountSecretSettings :one
SELECT count(*)
FROM secret_settings;

-- name: GetEncryptionKeyFingerprint :one
SELECT key_fingerprint
FROM encryption_key_state
WHERE singleton = true;

-- name: RegisterEncryptionKeyFingerprint :exec
INSERT INTO encryption_key_state (singleton, key_fingerprint)
VALUES (true, $1)
ON CONFLICT (singleton) DO NOTHING;

-- name: GetEncryptionKeyFingerprintForUpdate :one
SELECT key_fingerprint
FROM encryption_key_state
WHERE singleton = true
FOR UPDATE;
