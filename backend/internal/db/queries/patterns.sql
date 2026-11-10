-- Win patterns and their categories. A pattern is a name, a 5x5 grid kept as JSON
-- (pattern_data, a JSON array of five arrays of five booleans), its category and its
-- place within it (sort_order, lowest first). Categories are ordered the same way.

-- name: ListPatternCategories :many
SELECT id, name, sort_order FROM pattern_categories ORDER BY sort_order, id;

-- name: GetPatternCategory :one
SELECT id, name, sort_order FROM pattern_categories WHERE id = ?;

-- name: FallbackPatternCategory :one
-- The category a deleted one's patterns move to: the first of the others.
SELECT id FROM pattern_categories WHERE id != ? ORDER BY sort_order, id LIMIT 1;

-- name: CreatePatternCategory :one
INSERT INTO pattern_categories (name, sort_order)
VALUES (sqlc.arg(name), (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM pattern_categories))
RETURNING id, name, sort_order;

-- name: RenamePatternCategory :one
UPDATE pattern_categories SET name = ? WHERE id = ?
RETURNING id, name, sort_order;

-- name: SetPatternCategoryOrder :execrows
UPDATE pattern_categories SET sort_order = ? WHERE id = ?;

-- name: DeletePatternCategory :execrows
DELETE FROM pattern_categories WHERE id = ?;

-- name: ListPatterns :many
SELECT p.id, p.name, p.pattern_data, p.sort_order, p.category_id, c.name AS category_name
FROM patterns p JOIN pattern_categories c ON c.id = p.category_id
ORDER BY c.sort_order, c.id, p.sort_order, p.id;

-- name: GetPattern :one
SELECT p.id, p.name, p.pattern_data, p.sort_order, p.category_id, c.name AS category_name
FROM patterns p JOIN pattern_categories c ON c.id = p.category_id
WHERE p.id = ?;

-- name: ListPatternsByIDs :many
-- The patterns among these ids that still exist, for a game to start with.
SELECT id, name, pattern_data FROM patterns WHERE id IN (sqlc.slice('ids')) ORDER BY id;

-- name: ListPatternIDsInCategory :many
SELECT id FROM patterns WHERE category_id = ? ORDER BY sort_order, id;

-- name: FindPatternByGrid :one
-- A pattern with the same grid, which a new one may not repeat.
SELECT p.name, c.name AS category_name
FROM patterns p JOIN pattern_categories c ON c.id = p.category_id
WHERE p.pattern_data = ?
LIMIT 1;

-- name: CreatePattern :one
-- A new pattern goes last in its category.
INSERT INTO patterns (name, pattern_data, category_id, sort_order)
VALUES (
    sqlc.arg(name), sqlc.arg(pattern_data), sqlc.arg(category_id),
    (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM patterns WHERE category_id = sqlc.arg(category_id))
)
RETURNING id;

-- name: RenamePattern :execrows
UPDATE patterns SET name = ? WHERE id = ?;

-- name: SetPatternOrder :execrows
-- Places a pattern in a category at a position (a reorder may move it across).
UPDATE patterns SET category_id = ?, sort_order = ? WHERE id = ?;

-- name: LastPatternOrder :one
SELECT CAST(COALESCE(MAX(sort_order), 0) AS INTEGER) FROM patterns WHERE category_id = ?;

-- name: RehomePatterns :exec
-- Moves every pattern of a category after another's, keeping their order.
UPDATE patterns SET category_id = sqlc.arg(to_category), sort_order = sort_order + sqlc.arg(after)
WHERE category_id = sqlc.arg(from_category);

-- name: DeletePattern :execrows
DELETE FROM patterns WHERE id = ?;
