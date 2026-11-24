-- name: CreateCredential :one
INSERT INTO credentials (
  name, kind, secret, host, port, username, host_key_fingerprint, created_by
)
VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetCredential :one
SELECT * FROM credentials WHERE id = $1;

-- name: GetCredentialByName :one
SELECT * FROM credentials WHERE name = $1;

-- name: ListCredentials :many
-- Never selects the secret: this row feeds the API, and a struct that cannot
-- carry the ciphertext cannot leak it.
SELECT id, name, kind, host, port, username, host_key_fingerprint, rotated_at, last_used_at, created_by, created_at
FROM credentials
ORDER BY name;

-- name: RotateCredential :one
UPDATE credentials
SET secret = $2, rotated_at = now(), updated_at = now()
WHERE id = $1
RETURNING id, rotated_at;

-- name: TouchCredential :exec
UPDATE credentials SET last_used_at = now() WHERE id = $1;

-- name: DeleteCredential :exec
DELETE FROM credentials WHERE id = $1;
