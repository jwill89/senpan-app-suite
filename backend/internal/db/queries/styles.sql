-- Themes: a name, design tokens (a JSON object of CSS custom properties), the two
-- flourish images, and whether players may pick it.

-- name: ListStyles :many
SELECT id, name, is_public, created_at FROM styles ORDER BY name, id;

-- name: ListPublicStyles :many
SELECT id, name FROM styles WHERE is_public = 1 ORDER BY name, id;

-- name: GetStyle :one
SELECT id, name, tokens, board_flourish, number_flourish, is_public, created_at FROM styles WHERE id = ?;

-- name: GetPublicStyle :one
SELECT id, name, tokens, board_flourish, number_flourish, is_public, created_at FROM styles WHERE id = ? AND is_public = 1;

-- name: CreateStyle :one
INSERT INTO styles (name, tokens, board_flourish, number_flourish, is_public)
VALUES (?, ?, ?, ?, ?)
RETURNING id, name, tokens, board_flourish, number_flourish, is_public, created_at;

-- name: UpdateStyle :one
UPDATE styles SET name = ?, tokens = ?, board_flourish = ?, number_flourish = ?, is_public = ?
WHERE id = ?
RETURNING id, name, tokens, board_flourish, number_flourish, is_public, created_at;

-- name: DeleteStyle :execrows
DELETE FROM styles WHERE id = ?;
