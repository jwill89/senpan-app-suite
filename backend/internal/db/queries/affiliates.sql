-- Affiliates: partner establishments. owners and hours are JSON arrays kept in their
-- columns; sort_order is the manual drag order.

-- name: ListAffiliates :many
SELECT id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot,
  embed_color, discord_link, carrd_link, sort_order, created_at
FROM affiliates ORDER BY sort_order ASC, name COLLATE NOCASE ASC, id ASC;

-- name: GetAffiliateByID :one
SELECT id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot,
  embed_color, discord_link, carrd_link, sort_order, created_at
FROM affiliates WHERE id = ?;

-- name: CreateAffiliate :one
INSERT INTO affiliates
  (name, subtitle, owners, location, timezone, hours, details, logo, screenshot, embed_color, discord_link, carrd_link)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot,
  embed_color, discord_link, carrd_link, sort_order, created_at;

-- sort_order is left alone: an edit must not reset the drag order.
-- name: UpdateAffiliate :one
UPDATE affiliates SET name = ?, subtitle = ?, owners = ?, location = ?, timezone = ?,
  hours = ?, details = ?, logo = ?, screenshot = ?, embed_color = ?, discord_link = ?, carrd_link = ?
WHERE id = ?
RETURNING id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot,
  embed_color, discord_link, carrd_link, sort_order, created_at;

-- name: SetAffiliateOrder :execrows
UPDATE affiliates SET sort_order = ? WHERE id = ?;

-- A pitch occupant may name an affiliate and nothing else (a blank title). Its link
-- clears when the affiliate is deleted, so the name is copied onto the title first.
-- name: NameOccupantsAfterAffiliate :exec
UPDATE festival_stall_occupants
SET title = COALESCE((SELECT a.name FROM affiliates a WHERE a.id = sqlc.arg(affiliate_id)), title)
WHERE affiliate_id = sqlc.arg(affiliate_id) AND TRIM(title) = '';

-- name: DeleteAffiliate :execrows
DELETE FROM affiliates WHERE id = ?;
