-- Book clubs: each club's named reading lists and their ordered items. A list
-- belongs to one club, and every query that takes a list's club or an item's list
-- scopes by it, so an id from another club's page finds nothing.

-- name: ListReadingLists :many
SELECT id, club_slug, title, created_at FROM reading_lists
WHERE club_slug = ? ORDER BY created_at DESC, id DESC;

-- name: GetReadingList :one
SELECT id, club_slug, title, created_at FROM reading_lists WHERE id = ? AND club_slug = ?;

-- name: CreateReadingList :one
INSERT INTO reading_lists (club_slug, title) VALUES (?, ?)
RETURNING id, club_slug, title, created_at;

-- name: UpdateReadingListTitle :one
UPDATE reading_lists SET title = ? WHERE id = ? AND club_slug = ?
RETURNING id, club_slug, title, created_at;

-- Its items go with it (the foreign key cascades).
-- name: DeleteReadingList :execrows
DELETE FROM reading_lists WHERE id = ? AND club_slug = ?;

-- name: ListReadingListItems :many
SELECT id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order
FROM reading_list_items WHERE list_id = ? ORDER BY sort_order ASC, id ASC;

-- name: GetReadingListItem :one
SELECT id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order
FROM reading_list_items WHERE id = ? AND list_id = ?;

-- A new item goes to the end of its list.
-- name: CreateReadingListItem :one
INSERT INTO reading_list_items
  (list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order)
VALUES (sqlc.arg(list_id), sqlc.arg(cover_image), sqlc.arg(title), sqlc.arg(summary), sqlc.arg(format),
  sqlc.arg(genres), sqlc.arg(tropes), sqlc.arg(chapters), sqlc.arg(comments), sqlc.arg(sources),
  (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM reading_list_items WHERE list_id = sqlc.arg(list_id)))
RETURNING id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order;

-- Its place in the list is left alone.
-- name: UpdateReadingListItem :one
UPDATE reading_list_items SET cover_image = ?, title = ?, summary = ?, format = ?, genres = ?,
  tropes = ?, chapters = ?, comments = ?, sources = ?
WHERE id = ? AND list_id = ?
RETURNING id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order;

-- name: DeleteReadingListItem :execrows
DELETE FROM reading_list_items WHERE id = ? AND list_id = ?;

-- name: CountReadingListItemsByCover :one
SELECT COUNT(*) FROM reading_list_items WHERE cover_image = ?;
