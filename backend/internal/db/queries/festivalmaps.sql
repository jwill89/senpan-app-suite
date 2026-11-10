-- Festival maps: a floor plan whose pitches are drawn over a base image. A pitch holds
-- the occupants standing in it (several when a booth changes hands between days). A
-- map's times, and an occupant's, are JSON arrays kept in their columns.

-- name: ListFestivalMaps :many
SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.status, m.created_at,
       CAST((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id) AS INTEGER) AS stall_count
FROM festival_maps m ORDER BY m.created_at DESC, m.id DESC;

-- name: GetFestivalMap :one
SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.status, m.created_at,
       CAST((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id) AS INTEGER) AS stall_count
FROM festival_maps m WHERE m.id = ?;

-- name: GetFestivalMapBySlug :one
-- The empty shortcode means none, so the caller never asks for it.
SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.status, m.created_at,
       CAST((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id) AS INTEGER) AS stall_count
FROM festival_maps m WHERE m.slug = ?;

-- name: ListPublishedFestivalMaps :many
SELECT m.id, m.title, m.slug, m.description, m.times, m.map_image, m.status, m.created_at,
       CAST((SELECT COUNT(*) FROM festival_stalls st WHERE st.map_id = m.id) AS INTEGER) AS stall_count
FROM festival_maps m WHERE m.status = 'published' ORDER BY m.created_at DESC, m.id DESC;

-- name: CreateFestivalMap :one
INSERT INTO festival_maps (title, slug, description, times, map_image, status)
VALUES (?, ?, ?, ?, ?, 'in_progress')
RETURNING id;

-- A map's status changes through its own command.
-- name: UpdateFestivalMap :execrows
UPDATE festival_maps SET title = ?, slug = ?, description = ?, times = ?, map_image = ? WHERE id = ?;

-- name: SetFestivalMapStatus :execrows
UPDATE festival_maps SET status = ? WHERE id = ?;

-- name: DeleteFestivalMap :execrows
DELETE FROM festival_maps WHERE id = ?;

-- The links to a map are plain columns, not enforced keys, so deleting a map clears
-- them: the rallies, raffles and stamps that named it survive.

-- name: ClearMapStampOccupants :exec
UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id IN
  (SELECT o.id FROM festival_stall_occupants o
     JOIN festival_stalls st ON st.id = o.stall_id WHERE st.map_id = ?);

-- name: ClearMapRaffles :exec
UPDATE raffles SET festival_map_id = NULL, occupant_id = NULL WHERE festival_map_id = ?;

-- name: ClearMapRallies :exec
UPDATE stamp_rallies SET festival_map_id = NULL WHERE festival_map_id = ?;

-- name: ListFestivalStalls :many
SELECT id, map_id, shape, color, selection_color, text_color, pos_x, pos_y, width, height, rotation, sort_order
FROM festival_stalls WHERE map_id = ? ORDER BY sort_order ASC, id ASC;

-- name: ListMapOccupants :many
-- Every occupant on a map's pitches, in display order, with the affiliate's name.
SELECT o.id, o.stall_id, o.affiliate_id, CAST(COALESCE(a.name, '') AS TEXT) AS affiliate_name,
       o.title, o.description, o.event_carrd, o.stall_type, o.type_label, o.times, o.sort_order
FROM festival_stall_occupants o
JOIN festival_stalls st ON st.id = o.stall_id
LEFT JOIN affiliates a ON a.id = o.affiliate_id
WHERE st.map_id = ?
ORDER BY o.sort_order ASC, o.id ASC;

-- name: InsertFestivalStall :one
INSERT INTO festival_stalls (map_id, shape, color, selection_color, text_color, pos_x, pos_y, width, height, rotation, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- Scoped to the map as well as the pitch, and the row count checked: a save of one map
-- carrying another map's pitch id must not reach that pitch's occupants.
-- name: UpdateFestivalStall :execrows
UPDATE festival_stalls SET shape = ?, color = ?, selection_color = ?, text_color = ?,
       pos_x = ?, pos_y = ?, width = ?, height = ?, rotation = ?, sort_order = ?
WHERE id = ? AND map_id = ?;

-- name: ListMapStallIDs :many
SELECT id FROM festival_stalls WHERE map_id = ?;

-- name: ClearStallStampOccupants :exec
UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id IN
  (SELECT id FROM festival_stall_occupants WHERE stall_id = ?);

-- A raffle assigned to an occupant keeps its map, since it is still part of the
-- festival, but loses the pitch.
-- name: ClearStallRaffleOccupants :exec
UPDATE raffles SET occupant_id = NULL WHERE occupant_id IN
  (SELECT id FROM festival_stall_occupants WHERE stall_id = ?);

-- name: DeleteFestivalStall :exec
DELETE FROM festival_stalls WHERE id = ?;

-- name: ListStallOccupantIDs :many
SELECT id FROM festival_stall_occupants WHERE stall_id = ?;

-- name: InsertStallOccupant :one
INSERT INTO festival_stall_occupants
  (stall_id, affiliate_id, title, description, event_carrd, stall_type, type_label, times, sort_order)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: UpdateStallOccupant :execrows
UPDATE festival_stall_occupants
SET affiliate_id = ?, title = ?, description = ?, event_carrd = ?, stall_type = ?, type_label = ?, times = ?, sort_order = ?
WHERE id = ? AND stall_id = ?;

-- name: ClearOccupantStamps :exec
UPDATE stamp_rally_stamps SET occupant_id = NULL WHERE occupant_id = ?;

-- name: ClearOccupantRaffles :exec
UPDATE raffles SET occupant_id = NULL WHERE occupant_id = ?;

-- name: DeleteStallOccupant :exec
DELETE FROM festival_stall_occupants WHERE id = ?;

-- name: GetRallyFestival :one
-- The published festival a rally is filed under, for linking to its page.
SELECT m.id, m.slug FROM stamp_rallies r
JOIN festival_maps m ON m.id = r.festival_map_id
WHERE r.id = ? AND m.status = 'published';

-- name: GetOpenRallyForMap :one
-- The newest stamp rally of a map that isn't closed. Whether it is running now is the
-- caller's question, against the clock.
SELECT id, title, public_signup, available_from, available_to
FROM stamp_rallies WHERE festival_map_id = ? AND status != 'closed' ORDER BY id DESC LIMIT 1;

-- name: ListRallyStampImagesForMap :many
-- The stamps a rally placed on pitch occupants; one that names no occupant is left out.
SELECT occupant_id, image FROM stamp_rally_stamps WHERE rally_id = ? AND occupant_id IS NOT NULL;

-- name: ListStallRaffles :many
-- The raffles assigned to a map's occupants, oldest first, so a pitch with more than
-- one keeps the newest (the stall panel links to one).
SELECT r.occupant_id, r.id, r.title, r.prize_image, r.status, r.available_from, r.available_to
FROM raffles r
JOIN festival_stall_occupants o ON o.id = r.occupant_id
JOIN festival_stalls st ON st.id = o.stall_id
WHERE st.map_id = ? AND r.occupant_id IS NOT NULL
ORDER BY r.created_at ASC, r.id ASC;

-- name: ListFestivalRaffles :many
-- Every raffle filed under a festival, newest first.
SELECT r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions, r.entry_mode,
       r.cost_per_entry, r.tier_costs, r.available_from, r.available_to, r.prize_image, r.pay_image,
       r.status, r.winner_entry_id, r.festival_map_id, CAST(COALESCE(fm.title, '') AS TEXT) AS festival_map_name,
       r.occupant_id, CAST(COALESCE(fo.title, '') AS TEXT) AS stall_name, r.created_at
FROM raffles r
LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
LEFT JOIN festival_stall_occupants fo ON fo.id = r.occupant_id
WHERE r.festival_map_id = ?
ORDER BY r.created_at DESC, r.id DESC;

-- name: ListFestivalRaffleTallies :many
-- Each of a festival's raffles with its ticket total and its winner's name.
SELECT r.id,
       CAST(COALESCE((SELECT SUM(e.num_entries) FROM raffle_entries e WHERE e.raffle_id = r.id), 0) AS INTEGER) AS total_entries,
       CAST(COALESCE(w.character_name, '') AS TEXT) AS winner_name,
       CAST(COALESCE(w.world, '') AS TEXT) AS winner_world,
       CAST(COALESCE(w.num_entries, 0) AS INTEGER) AS winner_tickets
FROM raffles r LEFT JOIN raffle_entries w ON w.id = r.winner_entry_id
WHERE r.festival_map_id = ?;

-- name: ListFestivalRaffleEntriesFor :many
-- One participant's entries across a festival's raffles, on the whole name and world
-- (the identity entries merge on), ignoring case: never a substring.
SELECT e.raffle_id, e.character_name, e.world, e.num_entries, e.paid_entries
FROM raffle_entries e JOIN raffles r ON r.id = e.raffle_id
WHERE r.festival_map_id = sqlc.arg(map_id)
  AND LOWER(e.character_name) = LOWER(sqlc.arg(name)) AND LOWER(e.world) = LOWER(sqlc.arg(world));

-- name: GetGaraponPlayerByCard :one
-- The drawing link issued with a stamp card.
SELECT p.id, p.garapon_id, p.token, p.player_name, p.world, p.max_draws, p.created_at,
       CAST((SELECT COUNT(*) FROM garapon_draws d WHERE d.player_id = p.id) AS INTEGER) AS draws_used,
       CAST(COALESCE(sc.token, '') AS TEXT) AS stamp_card_token
FROM garapon_players p
LEFT JOIN stamp_rally_cards sc ON sc.id = p.stamp_card_id
WHERE p.stamp_card_id = ?;
