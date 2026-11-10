-- +goose Up
-- The artist's request queue joined the Ema Plaques page, so its page permission is gone.
-- An account that held it gets the Ema Plaques one in its place (the same page now, in the
-- same spot in its list, once), and the old key leaves every list.
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
