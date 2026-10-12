-- name: GetPrincipal :one
SELECT id,
       principal_type,
       status,
       COALESCE(email, '') AS email,
       display_name,
       disabled_at,
       blocked_at,
       last_seen_at,
       created_at,
       updated_at
FROM access.principal
WHERE id = sqlc.arg(principal_id)::uuid AND revoked_at IS NULL;

-- name: GetPrincipalForUpdate :one
SELECT id,
       principal_type,
       status,
       COALESCE(email, '') AS email,
       display_name,
       disabled_at,
       blocked_at,
       last_seen_at,
       created_at,
       updated_at
FROM access.principal
WHERE id = sqlc.arg(principal_id)::uuid AND revoked_at IS NULL
FOR UPDATE;

-- name: ListPrincipals :many
SELECT id,
       principal_type,
       status,
       COALESCE(email, '') AS email,
       display_name,
       disabled_at,
       blocked_at,
       last_seen_at,
       created_at,
       updated_at
FROM access.principal
WHERE revoked_at IS NULL
  AND (sqlc.arg(email)::text = '' OR lower(email) = lower(sqlc.arg(email)::text))
  AND (sqlc.arg(query)::text = '' OR email ILIKE '%' || sqlc.arg(query)::text || '%' OR display_name ILIKE '%' || sqlc.arg(query)::text || '%')
ORDER BY created_at DESC
LIMIT sqlc.arg(page_size)::int;

-- Admin-only activity projection; safe metadata readers keep ListPrincipals.
-- name: ListPrincipalsWithActivity :many
SELECT p.id,
       principal_type,
       status,
       COALESCE(email, '') AS email,
       display_name,
       disabled_at,
       blocked_at,
       GREATEST(p.last_seen_at, (
         SELECT MAX(COALESCE(s.last_seen_at, s.created_at))
         FROM access.session s WHERE s.principal_id = p.id
       ))::timestamptz AS last_seen_at,
       created_at,
       updated_at
FROM access.principal p
WHERE revoked_at IS NULL
  AND (sqlc.arg(email)::text = '' OR lower(email) = lower(sqlc.arg(email)::text))
  AND (sqlc.arg(query)::text = '' OR email ILIKE '%' || sqlc.arg(query)::text || '%' OR display_name ILIKE '%' || sqlc.arg(query)::text || '%')
ORDER BY created_at DESC
LIMIT sqlc.arg(page_size)::int;
