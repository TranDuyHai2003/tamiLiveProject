-- name: CreateUser :one
INSERT INTO users (username, email, password_hash, role)
VALUES ($1, $2, $3, $4)
RETURNING id, username, email, role, status, level, avatar_url, created_at;

-- name: GetUserByID :one
SELECT id, username, email, role, status, level, avatar_url, created_at
FROM users
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetUserCredentialsByEmail :one
SELECT id, password_hash, status
FROM users
WHERE LOWER(email) = LOWER(sqlc.arg(email)) AND deleted_at IS NULL;
