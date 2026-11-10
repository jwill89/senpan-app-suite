-- Raffles and their entries. A raffle's prices are whole gil kept in REAL columns
-- (cost_per_entry, a JSON array in tier_costs, and an entry's amount_waived); its
-- times are the stored UTC form or NULL. An entry's tickets are num_entries, of which
-- paid_entries are settled; paid is the derived "nothing outstanding" flag.

-- name: ListRaffles :many
SELECT r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions, r.entry_mode,
       r.cost_per_entry, r.tier_costs, r.available_from, r.available_to, r.prize_image, r.pay_image,
       r.status, r.winner_entry_id, r.festival_map_id, CAST(COALESCE(fm.title, '') AS TEXT) AS festival_map_name,
       r.occupant_id, CAST(COALESCE(fo.title, '') AS TEXT) AS stall_name, r.created_at
FROM raffles r
LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
LEFT JOIN festival_stall_occupants fo ON fo.id = r.occupant_id
ORDER BY r.created_at DESC, r.id DESC;

-- name: GetRaffle :one
SELECT r.id, r.title, r.description, r.rules, r.max_entries, r.signup_instructions, r.entry_mode,
       r.cost_per_entry, r.tier_costs, r.available_from, r.available_to, r.prize_image, r.pay_image,
       r.status, r.winner_entry_id, r.festival_map_id, CAST(COALESCE(fm.title, '') AS TEXT) AS festival_map_name,
       r.occupant_id, CAST(COALESCE(fo.title, '') AS TEXT) AS stall_name, r.created_at
FROM raffles r
LEFT JOIN festival_maps fm ON fm.id = r.festival_map_id
LEFT JOIN festival_stall_occupants fo ON fo.id = r.occupant_id
WHERE r.id = ?;

-- name: CreateRaffle :one
INSERT INTO raffles (title, description, rules, max_entries, signup_instructions, entry_mode, cost_per_entry,
                     tier_costs, available_from, available_to, prize_image, pay_image, status, festival_map_id, occupant_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open', ?, ?)
RETURNING id;

-- name: UpdateRaffle :execrows
-- Everything a raffle's form edits; its status and winner are not.
UPDATE raffles
SET title = ?, description = ?, rules = ?, max_entries = ?, signup_instructions = ?, entry_mode = ?,
    cost_per_entry = ?, tier_costs = ?, available_from = ?, available_to = ?, prize_image = ?, pay_image = ?,
    festival_map_id = ?, occupant_id = ?
WHERE id = ?;

-- name: DeleteRaffle :execrows
-- Its entries go with it (cascade).
DELETE FROM raffles WHERE id = ?;

-- name: SetRaffleStatus :execrows
UPDATE raffles SET status = ? WHERE id = ?;

-- name: PickRaffleWinner :execrows
-- Names the pending winner only where none is set, so two consoles picking at once
-- cannot both succeed.
UPDATE raffles SET winner_entry_id = ? WHERE id = ? AND winner_entry_id IS NULL;

-- name: ClearRaffleWinner :exec
UPDATE raffles SET winner_entry_id = NULL WHERE id = ?;

-- name: ClearRaffleWinnerIfEntry :exec
-- The raffle's winner column is not an enforced foreign key, so deleting the entry it
-- names has to clear it.
UPDATE raffles SET winner_entry_id = NULL WHERE winner_entry_id = ?;

-- name: ListRaffleEntries :many
SELECT id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at
FROM raffle_entries WHERE raffle_id = ? ORDER BY created_at, id;

-- name: GetRaffleEntry :one
SELECT id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at
FROM raffle_entries WHERE id = ?;

-- name: FindRaffleEntry :one
-- A character's entry in a raffle: entries merge on the whole name and world, ignoring case.
SELECT id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at
FROM raffle_entries
WHERE raffle_id = ? AND LOWER(character_name) = LOWER(sqlc.arg(character_name)) AND LOWER(world) = LOWER(sqlc.arg(world));

-- name: CreateRaffleEntry :one
INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries) VALUES (?, ?, ?, ?)
RETURNING id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at;

-- name: AddRaffleEntryTickets :exec
-- More tickets on a row. They are unsettled, so the paid flag drops back off unless
-- everything on the row is still settled; paid_entries and amount_waived stay, what
-- was settled and forgiven still was.
UPDATE raffle_entries
SET num_entries = num_entries + sqlc.arg(tickets),
    paid = CASE WHEN paid_entries >= num_entries + sqlc.arg(tickets) THEN 1 ELSE 0 END
WHERE id = sqlc.arg(id);

-- name: SettleAllRaffleEntryTickets :execrows
-- Settles every ticket on a row and adds to what was waived. It only moves forward
-- (a row with nothing outstanding changes nothing), which makes it idempotent: two
-- staff settling the same entry cannot stack one waiver twice.
UPDATE raffle_entries
SET paid = 1, paid_entries = num_entries, amount_waived = amount_waived + sqlc.arg(waived)
WHERE id = sqlc.arg(id) AND paid_entries < num_entries;

-- name: SettleSomeRaffleEntryTickets :execrows
-- Settles up to tickets of a row's tickets, forward only like SettleAllRaffleEntryTickets.
-- The count is clamped in the statement: num_entries can grow between a read and this
-- write, and only the statement sees the row's real count.
UPDATE raffle_entries
SET paid_entries = MIN(CAST(sqlc.arg(tickets) AS INTEGER), num_entries),
    paid = CASE WHEN MIN(CAST(sqlc.arg(tickets) AS INTEGER), num_entries) >= num_entries THEN 1 ELSE 0 END,
    amount_waived = amount_waived + sqlc.arg(waived)
WHERE id = sqlc.arg(id) AND MIN(CAST(sqlc.arg(tickets) AS INTEGER), num_entries) > paid_entries;

-- name: ClearRaffleEntrySettlement :execrows
-- Nothing settled and nothing waived, so a mis-click leaves no waiver credited against
-- an unpaid entry.
UPDATE raffle_entries SET paid = 0, paid_entries = 0, amount_waived = 0 WHERE id = ?;

-- name: DeleteRaffleEntry :execrows
DELETE FROM raffle_entries WHERE id = ?;

-- name: CountRaffleTickets :one
SELECT CAST(COALESCE(SUM(num_entries), 0) AS INTEGER) FROM raffle_entries WHERE raffle_id = ?;

-- name: ListRaffleWinnerNames :many
-- The entry each raffle's winner column names.
SELECT e.id, e.character_name, e.world FROM raffle_entries e JOIN raffles r ON r.winner_entry_id = e.id;

-- name: ListSettledRaffleEntries :many
-- The settlement of each entry that has paid for at least one ticket, kept apart
-- because a tiered raffle prices one entry of three tickets differently from three
-- entries of one, and each carries its own waiver.
SELECT raffle_id, num_entries, paid_entries, amount_waived FROM raffle_entries WHERE paid_entries > 0;

-- name: SearchRaffleEntries :many
-- A raffle's entries whose character name contains the search, ignoring case. INSTR
-- takes the search literally, where LIKE would read "%" and "_" in it as wildcards.
-- Taking one row past the limit says there are more.
SELECT character_name, world, num_entries, paid_entries
FROM raffle_entries
WHERE raffle_id = sqlc.arg(raffle_id) AND INSTR(LOWER(character_name), LOWER(sqlc.arg(search))) > 0
ORDER BY character_name COLLATE NOCASE, world COLLATE NOCASE
LIMIT sqlc.arg(row_limit);
