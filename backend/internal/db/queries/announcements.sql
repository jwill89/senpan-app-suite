-- Announcements: Discord embeds posted by hand or on a schedule, to the webhook of
-- their type. The types and the taggable roles are small lists beside them.

-- name: ListAnnouncementTypes :many
SELECT id, name, webhook_url, created_at FROM announcement_types ORDER BY id DESC;

-- name: GetAnnouncementType :one
SELECT id, name, webhook_url, created_at FROM announcement_types WHERE id = ?;

-- name: CreateAnnouncementType :one
INSERT INTO announcement_types (name, webhook_url) VALUES (?, ?)
RETURNING id, name, webhook_url, created_at;

-- name: UpdateAnnouncementType :one
UPDATE announcement_types SET name = ?, webhook_url = ? WHERE id = ?
RETURNING id, name, webhook_url, created_at;

-- name: DeleteAnnouncementType :execrows
DELETE FROM announcement_types WHERE id = ?;

-- name: CountAnnouncementsByType :one
SELECT COUNT(*) FROM announcements WHERE type_id = ?;

-- name: ListAnnouncementRoles :many
SELECT id, name, role_id, created_at FROM announcement_roles ORDER BY id DESC;

-- name: GetAnnouncementRole :one
SELECT id, name, role_id, created_at FROM announcement_roles WHERE id = ?;

-- name: CreateAnnouncementRole :one
INSERT INTO announcement_roles (name, role_id) VALUES (?, ?)
RETURNING id, name, role_id, created_at;

-- name: UpdateAnnouncementRole :one
UPDATE announcement_roles SET name = ?, role_id = ? WHERE id = ?
RETURNING id, name, role_id, created_at;

-- name: DeleteAnnouncementRole :execrows
DELETE FROM announcement_roles WHERE id = ?;

-- An announcement tags a role by "role:<id>" in its mention column.
-- name: CountAnnouncementsByMention :one
SELECT COUNT(*) FROM announcements WHERE mention = ?;

-- name: ListAnnouncements :many
-- In the staff's order, newest first among those not yet placed (sort_order 0).
SELECT id, type_id, title, details, image, thumbnail, color, location, start_local, end_local,
  start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local,
  schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active,
  last_posted_at, created_at, buttons, mention, sort_order
FROM announcements ORDER BY sort_order ASC, id DESC;

-- name: GetAnnouncement :one
SELECT id, type_id, title, details, image, thumbnail, color, location, start_local, end_local,
  start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local,
  schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active,
  last_posted_at, created_at, buttons, mention, sort_order
FROM announcements WHERE id = ?;

-- name: CreateAnnouncement :one
INSERT INTO announcements
  (type_id, title, details, image, thumbnail, color, location, start_local, end_local, start_at, end_at,
   start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local, schedule_minutes,
   schedule_weekdays, schedule_week_of_month, next_post_at, active, buttons, mention)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, type_id, title, details, image, thumbnail, color, location, start_local, end_local,
  start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local,
  schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active,
  last_posted_at, created_at, buttons, mention, sort_order;

-- skip_count and last_posted_at are left alone: the first belongs to the schedule's
-- cursor and the skip operation, the second is stamped only by a post.
-- name: UpdateAnnouncement :one
UPDATE announcements SET type_id = ?, title = ?, details = ?, image = ?, thumbnail = ?, color = ?,
  location = ?, start_local = ?, end_local = ?, start_at = ?, end_at = ?, start_format = ?,
  end_format = ?, dynamic_dates = ?, schedule_kind = ?, timezone = ?, once_local = ?,
  schedule_minutes = ?, schedule_weekdays = ?, schedule_week_of_month = ?, next_post_at = ?,
  active = ?, buttons = ?, mention = ?
WHERE id = ?
RETURNING id, type_id, title, details, image, thumbnail, color, location, start_local, end_local,
  start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local,
  schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active,
  last_posted_at, created_at, buttons, mention, sort_order;

-- name: SetAnnouncementOrder :execrows
UPDATE announcements SET sort_order = ? WHERE id = ?;

-- name: DeleteAnnouncement :execrows
DELETE FROM announcements WHERE id = ?;

-- name: DueAnnouncements :many
-- The scheduler's work queue: live announcements whose next post has arrived.
SELECT id, type_id, title, details, image, thumbnail, color, location, start_local, end_local,
  start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local,
  schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active,
  last_posted_at, created_at, buttons, mention, sort_order
FROM announcements WHERE active = 1 AND next_post_at <= ?
ORDER BY next_post_at ASC, id ASC;

-- name: MarkAnnouncementPosted :execrows
-- Stamps a post and moves the schedule's cursor. It applies only while the row still
-- holds the cursor the scheduler read before posting (prev_next_post_at): an edit made
-- during the post is the admin's and stands.
UPDATE announcements
SET last_posted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now'), next_post_at = sqlc.narg(next_post_at), active = sqlc.arg(active)
WHERE id = sqlc.arg(id) AND next_post_at IS sqlc.narg(prev_next_post_at);

-- name: AdvanceAnnouncement :execrows
-- Moves the cursor and the skips left without stamping a post (an occurrence passed
-- over), under the same guard as MarkAnnouncementPosted.
UPDATE announcements
SET next_post_at = sqlc.narg(next_post_at), active = sqlc.arg(active), skip_count = sqlc.arg(skip_count)
WHERE id = sqlc.arg(id) AND next_post_at IS sqlc.narg(prev_next_post_at);

-- name: SetAnnouncementSkip :execrows
UPDATE announcements SET skip_count = ? WHERE id = ?;

-- name: TouchAnnouncementPosted :execrows
-- Stamps a post made by hand without moving the schedule.
UPDATE announcements SET last_posted_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now') WHERE id = ?;
