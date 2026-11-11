-- +goose Up
-- The artist's request queue is part of the Ema Plaques page, so it has no page
-- permission of its own. An account that held festival-ema-requests gets festival-ema in
-- its place (at the same spot in its list, once), and the old key leaves every list.
UPDATE users
SET permissions = (
  SELECT json_group_array(v) FROM (
    SELECT CASE WHEN value = 'festival-ema-requests' THEN 'festival-ema' ELSE value END AS v, MIN(key) AS k
    FROM json_each(users.permissions)
    GROUP BY v
    ORDER BY k
  )
)
WHERE CASE WHEN json_valid(permissions)
  THEN EXISTS (SELECT 1 FROM json_each(users.permissions) WHERE value = 'festival-ema-requests')
  ELSE 0 END;
PRAGMA user_version = 75;
