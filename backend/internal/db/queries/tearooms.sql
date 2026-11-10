-- Tea rooms: the bookable rooms of the Tea House, in the order staff dragged them to
-- (sort_order, then newest first so a new room shows at the top until it is placed).
-- A room's lock may carry an expiry (locked_until); the flags are 0 or 1.

-- name: ListTeaRooms :many
SELECT id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
       seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at
FROM tea_rooms ORDER BY sort_order ASC, id DESC;

-- name: GetTeaRoom :one
SELECT id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
       seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at
FROM tea_rooms WHERE id = ?;

-- name: GetTeaRoomByNumber :one
SELECT id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
       seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at
FROM tea_rooms WHERE room_number = ? ORDER BY sort_order ASC, id ASC LIMIT 1;

-- name: CreateTeaRoom :one
INSERT INTO tea_rooms (name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
                       seasonal, open, lockable, discounted, locked, locked_until, image, color)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
          seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at;

-- name: UpdateTeaRoom :one
UPDATE tea_rooms SET name = ?, subtitle = ?, room_owner = ?, room_number = ?, cost_per_half_hour = ?, hashtags = ?,
    description = ?, seasonal = ?, open = ?, lockable = ?, discounted = ?, locked = ?, locked_until = ?,
    image = ?, color = ?
WHERE id = ?
RETURNING id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description,
          seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at;

-- name: SetTeaRoomOpen :exec
UPDATE tea_rooms SET open = ? WHERE id = ?;

-- name: SetTeaRoomDiscounted :exec
UPDATE tea_rooms SET discounted = ? WHERE id = ?;

-- name: SetTeaRoomLock :exec
UPDATE tea_rooms SET locked = ?, locked_until = ? WHERE id = ?;

-- name: SetTeaRoomOrder :execrows
UPDATE tea_rooms SET sort_order = ? WHERE id = ?;

-- name: DeleteTeaRoom :execrows
DELETE FROM tea_rooms WHERE id = ?;

-- name: ListExpiringTeaRoomLocks :many
-- Every room locked with an expiry set: the only locks that can lift on their own.
-- The caller compares the times, so an expiry that doesn't read as one is still seen.
SELECT id, name, room_number, locked_until
FROM tea_rooms WHERE locked = 1 AND locked_until IS NOT NULL ORDER BY id ASC;

-- name: ExpireTeaRoomLock :execrows
-- Lifts a lock that has run out, only while the room still holds the expiry the sweep
-- saw: a lock an admin re-timed or placed meanwhile is theirs and stays.
UPDATE tea_rooms SET locked = 0, locked_until = NULL
WHERE id = ? AND locked = 1 AND locked_until = ?;
