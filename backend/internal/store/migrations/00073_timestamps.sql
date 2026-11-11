-- +goose NO TRANSACTION
-- +goose Up
-- Every timestamp moves to one form: RFC 3339 in UTC to
-- the second, 2026-11-09T21:30:00Z, in a column declared DATETIME with a CHECK that
-- refuses any other, NULL where there is none (it was '' in the TEXT columns), and
-- strftime() in place of CURRENT_TIMESTAMP and datetime(), whose
-- '2026-11-09 21:30:00' sorts wrong against it. The values convert as they copy:
-- SQLite's space form, the browser's milliseconds (.000Z) and an offset all become
-- the one form in UTC, and the naive 2026-06-13T20:00 some raffles hold is read as
-- UTC, as the app reads it.
--
-- SQLite can't add a CHECK or change a type in place, so each table is rebuilt by
-- the procedure SQLite documents for it: foreign keys off
-- outside the transaction, a new table, the rows copied, the old one dropped and the
-- new one renamed, its indexes made again, AUTOINCREMENT's high-water mark kept so
-- no deleted id comes back, then foreign_key_check. A copy that drops a row or a
-- time, or a dangling reference, fails the CHECK on _migration_check, and the
-- transaction ends without committing, leaving every table as it was.
--
-- Afterwards production's schema is exactly a fresh build's. The hand-written
-- migrations had left it apart: columns in another order in sixteen tables, three
-- indexes missing, a full unique index on tea_rooms.room_number where a fresh
-- build has a partial one, cards.created_at without its default, and patterns
-- without its foreign key to pattern_categories. The rebuilds repair the tables
-- with timestamps; the four without (patterns, reading_list_items,
-- festival_stalls, festival_stall_occupants) are rebuilt for the rest.
PRAGMA foreign_keys = OFF;
BEGIN IMMEDIATE;
CREATE TEMP TABLE _migration_check (ok INTEGER NOT NULL CHECK (ok = 1), what TEXT NOT NULL);

-- affiliates
CREATE TABLE new_affiliates (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  owners TEXT NOT NULL DEFAULT '[]',
  location TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  hours TEXT NOT NULL DEFAULT '[]',
  details TEXT NOT NULL DEFAULT '',
  logo TEXT NOT NULL DEFAULT '',
  screenshot TEXT NOT NULL DEFAULT '',
  embed_color TEXT NOT NULL DEFAULT '',
  discord_link TEXT NOT NULL DEFAULT '',
  carrd_link TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_affiliates (id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot, embed_color, discord_link, carrd_link, sort_order, created_at)
  SELECT id, name, subtitle, owners, location, timezone, hours, details, logo, screenshot, embed_color, discord_link, carrd_link, sort_order, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM affiliates;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM affiliates) = (SELECT COUNT(*) FROM new_affiliates), 'affiliates: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM affiliates WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_affiliates WHERE created_at IS NOT NULL), 'affiliates.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_affiliates';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_affiliates', seq FROM sqlite_sequence WHERE name = 'affiliates';
DROP TABLE affiliates;
ALTER TABLE new_affiliates RENAME TO affiliates;
CREATE INDEX idx_affiliates_sort ON affiliates(sort_order, name COLLATE NOCASE, id);

-- announcement_roles
CREATE TABLE new_announcement_roles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  role_id TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_announcement_roles (id, name, role_id, created_at)
  SELECT id, name, role_id, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM announcement_roles;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcement_roles) = (SELECT COUNT(*) FROM new_announcement_roles), 'announcement_roles: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcement_roles WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcement_roles WHERE created_at IS NOT NULL), 'announcement_roles.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_announcement_roles';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_announcement_roles', seq FROM sqlite_sequence WHERE name = 'announcement_roles';
DROP TABLE announcement_roles;
ALTER TABLE new_announcement_roles RENAME TO announcement_roles;

-- announcement_types
CREATE TABLE new_announcement_types (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  webhook_url TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_announcement_types (id, name, webhook_url, created_at)
  SELECT id, name, webhook_url, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM announcement_types;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcement_types) = (SELECT COUNT(*) FROM new_announcement_types), 'announcement_types: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcement_types WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcement_types WHERE created_at IS NOT NULL), 'announcement_types.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_announcement_types';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_announcement_types', seq FROM sqlite_sequence WHERE name = 'announcement_types';
DROP TABLE announcement_types;
ALTER TABLE new_announcement_types RENAME TO announcement_types;

-- announcements
CREATE TABLE new_announcements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  type_id INTEGER NOT NULL,
  title TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL DEFAULT '',
  thumbnail TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '',
  location TEXT NOT NULL DEFAULT '',
  start_local TEXT NOT NULL DEFAULT '',
  end_local TEXT NOT NULL DEFAULT '',
  start_at DATETIME CHECK (start_at IS strftime('%Y-%m-%dT%H:%M:%SZ', start_at)),
  end_at DATETIME CHECK (end_at IS strftime('%Y-%m-%dT%H:%M:%SZ', end_at)),
  start_format TEXT NOT NULL DEFAULT '',
  end_format TEXT NOT NULL DEFAULT '',
  dynamic_dates INTEGER NOT NULL DEFAULT 0,
  schedule_kind TEXT NOT NULL DEFAULT '',
  timezone TEXT NOT NULL DEFAULT '',
  once_local TEXT NOT NULL DEFAULT '',
  schedule_minutes INTEGER NOT NULL DEFAULT 0,
  schedule_weekdays TEXT NOT NULL DEFAULT '',
  schedule_week_of_month INTEGER NOT NULL DEFAULT 0,
  next_post_at DATETIME CHECK (next_post_at IS strftime('%Y-%m-%dT%H:%M:%SZ', next_post_at)),
  skip_count INTEGER NOT NULL DEFAULT 0,
  active INTEGER NOT NULL DEFAULT 1,
  last_posted_at DATETIME CHECK (last_posted_at IS strftime('%Y-%m-%dT%H:%M:%SZ', last_posted_at)),
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  buttons TEXT NOT NULL DEFAULT '[]',
  mention TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (type_id) REFERENCES announcement_types(id)
);
INSERT INTO new_announcements (id, type_id, title, details, image, thumbnail, color, location, start_local, end_local, start_at, end_at, start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local, schedule_minutes, schedule_weekdays, schedule_week_of_month, next_post_at, skip_count, active, last_posted_at, created_at, buttons, mention, sort_order)
  SELECT id, type_id, title, details, image, thumbnail, color, location, start_local, end_local, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(start_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(end_at, '')), start_format, end_format, dynamic_dates, schedule_kind, timezone, once_local, schedule_minutes, schedule_weekdays, schedule_week_of_month, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(next_post_at, '')), skip_count, active, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(last_posted_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), buttons, mention, sort_order FROM announcements;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements) = (SELECT COUNT(*) FROM new_announcements), 'announcements: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements WHERE NULLIF(start_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcements WHERE start_at IS NOT NULL), 'announcements.start_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements WHERE NULLIF(end_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcements WHERE end_at IS NOT NULL), 'announcements.end_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements WHERE NULLIF(next_post_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcements WHERE next_post_at IS NOT NULL), 'announcements.next_post_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements WHERE NULLIF(last_posted_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcements WHERE last_posted_at IS NOT NULL), 'announcements.last_posted_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM announcements WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_announcements WHERE created_at IS NOT NULL), 'announcements.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_announcements';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_announcements', seq FROM sqlite_sequence WHERE name = 'announcements';
DROP TABLE announcements;
ALTER TABLE new_announcements RENAME TO announcements;
CREATE INDEX idx_announcements_due ON announcements(active, next_post_at);
CREATE INDEX idx_announcements_type ON announcements(type_id);

-- cards
CREATE TABLE new_cards (
  id TEXT PRIMARY KEY,
  board_data TEXT NOT NULL,
  player_name TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  protected INTEGER NOT NULL DEFAULT 0,
  custom_status TEXT NOT NULL DEFAULT '',
  world TEXT NOT NULL DEFAULT ''
);
INSERT INTO new_cards (id, board_data, player_name, details, created_at, protected, custom_status, world)
  SELECT id, board_data, player_name, details, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), protected, custom_status, world FROM cards;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM cards) = (SELECT COUNT(*) FROM new_cards), 'cards: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM cards WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_cards WHERE created_at IS NOT NULL), 'cards.created_at: times';
DROP TABLE cards;
ALTER TABLE new_cards RENAME TO cards;
CREATE INDEX idx_cards_player_name ON cards(player_name);

-- ema_boards
CREATE TABLE new_ema_boards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  festival_map_id INTEGER NOT NULL UNIQUE REFERENCES festival_maps(id) ON DELETE CASCADE,
  title TEXT NOT NULL DEFAULT '',
  template_image TEXT NOT NULL DEFAULT '',
  instructions TEXT NOT NULL DEFAULT '',
  discord_link TEXT NOT NULL DEFAULT '',
  allow_self INTEGER NOT NULL DEFAULT 1,
  allow_site INTEGER NOT NULL DEFAULT 1,
  allow_artist INTEGER NOT NULL DEFAULT 1,
  artist_name TEXT NOT NULL DEFAULT 'Tara',
  status TEXT NOT NULL DEFAULT 'open',
  created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_ema_boards (id, festival_map_id, title, template_image, instructions, discord_link, allow_self, allow_site, allow_artist, artist_name, status, created_at)
  SELECT id, festival_map_id, title, template_image, instructions, discord_link, allow_self, allow_site, allow_artist, artist_name, status, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM ema_boards;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_boards) = (SELECT COUNT(*) FROM new_ema_boards), 'ema_boards: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_boards WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_ema_boards WHERE created_at IS NOT NULL), 'ema_boards.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_ema_boards';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_ema_boards', seq FROM sqlite_sequence WHERE name = 'ema_boards';
DROP TABLE ema_boards;
ALTER TABLE new_ema_boards RENAME TO ema_boards;

-- ema_plaques
CREATE TABLE new_ema_plaques (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  board_id INTEGER NOT NULL REFERENCES ema_boards(id) ON DELETE CASCADE,
  window_id INTEGER REFERENCES ema_windows(id) ON DELETE SET NULL,
  window_label TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL,
  character_name TEXT NOT NULL,
  world TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL,
  reviewed_by TEXT NOT NULL DEFAULT '',
  reviewed_at DATETIME CHECK (reviewed_at IS strftime('%Y-%m-%dT%H:%M:%SZ', reviewed_at)),
  created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_ema_plaques (id, board_id, window_id, window_label, kind, character_name, world, description, image, status, reviewed_by, reviewed_at, created_at)
  SELECT id, board_id, window_id, window_label, kind, character_name, world, description, image, status, reviewed_by, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(reviewed_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM ema_plaques;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_plaques) = (SELECT COUNT(*) FROM new_ema_plaques), 'ema_plaques: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_plaques WHERE NULLIF(reviewed_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_ema_plaques WHERE reviewed_at IS NOT NULL), 'ema_plaques.reviewed_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_plaques WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_ema_plaques WHERE created_at IS NOT NULL), 'ema_plaques.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_ema_plaques';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_ema_plaques', seq FROM sqlite_sequence WHERE name = 'ema_plaques';
DROP TABLE ema_plaques;
ALTER TABLE new_ema_plaques RENAME TO ema_plaques;
CREATE INDEX idx_ema_plaques_board ON ema_plaques(board_id, status);
CREATE UNIQUE INDEX idx_ema_plaques_one_per_window ON ema_plaques(window_id, character_name COLLATE NOCASE, world COLLATE NOCASE) WHERE window_id IS NOT NULL AND status != 'rejected';

-- ema_windows
CREATE TABLE new_ema_windows (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  board_id INTEGER NOT NULL REFERENCES ema_boards(id) ON DELETE CASCADE,
  label TEXT NOT NULL DEFAULT '',
  start_at DATETIME NOT NULL CHECK (start_at IS strftime('%Y-%m-%dT%H:%M:%SZ', start_at)),
  end_at DATETIME NOT NULL CHECK (end_at IS strftime('%Y-%m-%dT%H:%M:%SZ', end_at)),
  webhook_url TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  artist_limit INTEGER NOT NULL DEFAULT 0
);
INSERT INTO new_ema_windows (id, board_id, label, start_at, end_at, webhook_url, sort_order, artist_limit)
  SELECT id, board_id, label, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(start_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(end_at, '')), webhook_url, sort_order, artist_limit FROM ema_windows;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_windows) = (SELECT COUNT(*) FROM new_ema_windows), 'ema_windows: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_windows WHERE NULLIF(start_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_ema_windows WHERE start_at IS NOT NULL), 'ema_windows.start_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM ema_windows WHERE NULLIF(end_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_ema_windows WHERE end_at IS NOT NULL), 'ema_windows.end_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_ema_windows';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_ema_windows', seq FROM sqlite_sequence WHERE name = 'ema_windows';
DROP TABLE ema_windows;
ALTER TABLE new_ema_windows RENAME TO ema_windows;
CREATE INDEX idx_ema_windows_board ON ema_windows(board_id);

-- festival_maps
CREATE TABLE new_festival_maps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  slug TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  times TEXT NOT NULL DEFAULT '[]',
  map_image TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'in_progress',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_festival_maps (id, title, slug, description, times, map_image, status, created_at)
  SELECT id, title, slug, description, times, map_image, status, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM festival_maps;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM festival_maps) = (SELECT COUNT(*) FROM new_festival_maps), 'festival_maps: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM festival_maps WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_festival_maps WHERE created_at IS NOT NULL), 'festival_maps.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_festival_maps';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_festival_maps', seq FROM sqlite_sequence WHERE name = 'festival_maps';
DROP TABLE festival_maps;
ALTER TABLE new_festival_maps RENAME TO festival_maps;
CREATE UNIQUE INDEX idx_festival_maps_slug ON festival_maps(slug) WHERE slug != '';
CREATE INDEX idx_festival_maps_status ON festival_maps(status);

-- game_presets
CREATE TABLE new_game_presets (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  pattern_ids TEXT NOT NULL DEFAULT '[]',
  game_details TEXT NOT NULL DEFAULT '',
  auto_call INTEGER NOT NULL DEFAULT 0,
  auto_interval INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_game_presets (id, name, pattern_ids, game_details, auto_call, auto_interval, created_at)
  SELECT id, name, pattern_ids, game_details, auto_call, auto_interval, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM game_presets;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM game_presets) = (SELECT COUNT(*) FROM new_game_presets), 'game_presets: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM game_presets WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_game_presets WHERE created_at IS NOT NULL), 'game_presets.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_game_presets';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_game_presets', seq FROM sqlite_sequence WHERE name = 'game_presets';
DROP TABLE game_presets;
ALTER TABLE new_game_presets RENAME TO game_presets;

-- games
CREATE TABLE new_games (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  status TEXT NOT NULL DEFAULT 'active',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  winners_cache TEXT NOT NULL DEFAULT '[]'
);
INSERT INTO new_games (id, status, created_at, winners_cache)
  SELECT id, status, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), winners_cache FROM games;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM games) = (SELECT COUNT(*) FROM new_games), 'games: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM games WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_games WHERE created_at IS NOT NULL), 'games.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_games';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_games', seq FROM sqlite_sequence WHERE name = 'games';
DROP TABLE games;
ALTER TABLE new_games RENAME TO games;
CREATE INDEX idx_games_status ON games(status);

-- garapon_draws
CREATE TABLE new_garapon_draws (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  garapon_id INTEGER NOT NULL,
  player_id INTEGER,
  prize_id INTEGER NOT NULL DEFAULT 0,
  player_name TEXT NOT NULL DEFAULT '',
  prize_name TEXT NOT NULL DEFAULT '',
  ball_color TEXT NOT NULL DEFAULT '',
  drawn_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (drawn_at IS strftime('%Y-%m-%dT%H:%M:%SZ', drawn_at)),
  FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE,
  FOREIGN KEY (player_id) REFERENCES garapon_players(id) ON DELETE SET NULL
);
INSERT INTO new_garapon_draws (id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at)
  SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(drawn_at, '')) FROM garapon_draws;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapon_draws) = (SELECT COUNT(*) FROM new_garapon_draws), 'garapon_draws: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapon_draws WHERE NULLIF(drawn_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_garapon_draws WHERE drawn_at IS NOT NULL), 'garapon_draws.drawn_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_garapon_draws';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_garapon_draws', seq FROM sqlite_sequence WHERE name = 'garapon_draws';
DROP TABLE garapon_draws;
ALTER TABLE new_garapon_draws RENAME TO garapon_draws;
CREATE INDEX idx_garapon_draws_garapon ON garapon_draws(garapon_id);
CREATE INDEX idx_garapon_draws_player ON garapon_draws(player_id);

-- garapon_players
CREATE TABLE new_garapon_players (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  garapon_id INTEGER NOT NULL,
  token TEXT NOT NULL UNIQUE,
  player_name TEXT NOT NULL DEFAULT '',
  max_draws INTEGER NOT NULL DEFAULT 1,
  stamp_card_id INTEGER,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  world TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE
);
INSERT INTO new_garapon_players (id, garapon_id, token, player_name, max_draws, stamp_card_id, created_at, world)
  SELECT id, garapon_id, token, player_name, max_draws, stamp_card_id, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), world FROM garapon_players;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapon_players) = (SELECT COUNT(*) FROM new_garapon_players), 'garapon_players: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapon_players WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_garapon_players WHERE created_at IS NOT NULL), 'garapon_players.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_garapon_players';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_garapon_players', seq FROM sqlite_sequence WHERE name = 'garapon_players';
DROP TABLE garapon_players;
ALTER TABLE new_garapon_players RENAME TO garapon_players;
CREATE INDEX idx_garapon_players_garapon ON garapon_players(garapon_id);
CREATE INDEX idx_garapon_players_stamp_card ON garapon_players(stamp_card_id);
CREATE INDEX idx_garapon_players_token ON garapon_players(token);

-- garapons
CREATE TABLE new_garapons (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  details TEXT NOT NULL DEFAULT '',
  grand_prize_image TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  stamp_rally_id INTEGER,
  default_draws INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_garapons (id, title, details, grand_prize_image, status, stamp_rally_id, default_draws, created_at)
  SELECT id, title, details, grand_prize_image, status, stamp_rally_id, default_draws, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM garapons;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapons) = (SELECT COUNT(*) FROM new_garapons), 'garapons: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM garapons WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_garapons WHERE created_at IS NOT NULL), 'garapons.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_garapons';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_garapons', seq FROM sqlite_sequence WHERE name = 'garapons';
DROP TABLE garapons;
ALTER TABLE new_garapons RENAME TO garapons;

-- raffle_entries
CREATE TABLE new_raffle_entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  raffle_id INTEGER NOT NULL,
  character_name TEXT NOT NULL,
  world TEXT NOT NULL,
  num_entries INTEGER NOT NULL DEFAULT 1,
  paid_entries INTEGER NOT NULL DEFAULT 0,
  amount_waived REAL NOT NULL DEFAULT 0,
  paid INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  FOREIGN KEY (raffle_id) REFERENCES raffles(id) ON DELETE CASCADE
);
INSERT INTO new_raffle_entries (id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, created_at)
  SELECT id, raffle_id, character_name, world, num_entries, paid_entries, amount_waived, paid, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM raffle_entries;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffle_entries) = (SELECT COUNT(*) FROM new_raffle_entries), 'raffle_entries: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffle_entries WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_raffle_entries WHERE created_at IS NOT NULL), 'raffle_entries.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_raffle_entries';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_raffle_entries', seq FROM sqlite_sequence WHERE name = 'raffle_entries';
DROP TABLE raffle_entries;
ALTER TABLE new_raffle_entries RENAME TO raffle_entries;
CREATE INDEX idx_raffle_entries_raffle ON raffle_entries(raffle_id);

-- raffles
CREATE TABLE new_raffles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  rules TEXT NOT NULL DEFAULT '',
  max_entries INTEGER NOT NULL DEFAULT 1,
  signup_instructions TEXT NOT NULL DEFAULT '',
  entry_mode TEXT NOT NULL DEFAULT 'single',
  cost_per_entry REAL NOT NULL DEFAULT 0,
  tier_costs TEXT NOT NULL DEFAULT '[]',
  available_from DATETIME CHECK (available_from IS strftime('%Y-%m-%dT%H:%M:%SZ', available_from)),
  available_to DATETIME CHECK (available_to IS strftime('%Y-%m-%dT%H:%M:%SZ', available_to)),
  prize_image TEXT NOT NULL DEFAULT '',
  pay_image TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  winner_entry_id INTEGER,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  festival_map_id INTEGER,
  occupant_id INTEGER
);
INSERT INTO new_raffles (id, title, description, rules, max_entries, signup_instructions, entry_mode, cost_per_entry, tier_costs, available_from, available_to, prize_image, pay_image, status, winner_entry_id, created_at, festival_map_id, occupant_id)
  SELECT id, title, description, rules, max_entries, signup_instructions, entry_mode, cost_per_entry, tier_costs, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(available_from, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(available_to, '')), prize_image, pay_image, status, winner_entry_id, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), festival_map_id, occupant_id FROM raffles;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffles) = (SELECT COUNT(*) FROM new_raffles), 'raffles: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffles WHERE NULLIF(available_from, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_raffles WHERE available_from IS NOT NULL), 'raffles.available_from: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffles WHERE NULLIF(available_to, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_raffles WHERE available_to IS NOT NULL), 'raffles.available_to: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM raffles WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_raffles WHERE created_at IS NOT NULL), 'raffles.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_raffles';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_raffles', seq FROM sqlite_sequence WHERE name = 'raffles';
DROP TABLE raffles;
ALTER TABLE new_raffles RENAME TO raffles;
CREATE INDEX idx_raffles_occupant ON raffles(occupant_id);

-- reading_lists
CREATE TABLE new_reading_lists (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  club_slug TEXT NOT NULL DEFAULT 'yaoi',
  title TEXT NOT NULL,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_reading_lists (id, club_slug, title, created_at)
  SELECT id, club_slug, title, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM reading_lists;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM reading_lists) = (SELECT COUNT(*) FROM new_reading_lists), 'reading_lists: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM reading_lists WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_reading_lists WHERE created_at IS NOT NULL), 'reading_lists.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_reading_lists';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_reading_lists', seq FROM sqlite_sequence WHERE name = 'reading_lists';
DROP TABLE reading_lists;
ALTER TABLE new_reading_lists RENAME TO reading_lists;

-- stamp_rallies
CREATE TABLE new_stamp_rallies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  card_image TEXT NOT NULL DEFAULT '',
  not_stamped_image TEXT NOT NULL DEFAULT '',
  available_from DATETIME CHECK (available_from IS strftime('%Y-%m-%dT%H:%M:%SZ', available_from)),
  available_to DATETIME CHECK (available_to IS strftime('%Y-%m-%dT%H:%M:%SZ', available_to)),
  details TEXT NOT NULL DEFAULT '',
  redeem_instructions TEXT NOT NULL DEFAULT '',
  redeem_image TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  public_signup INTEGER NOT NULL DEFAULT 0,
  completion_mode TEXT NOT NULL DEFAULT 'all',
  required_food INTEGER NOT NULL DEFAULT 0,
  required_game INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  festival_map_id INTEGER
);
INSERT INTO new_stamp_rallies (id, title, card_image, not_stamped_image, available_from, available_to, details, redeem_instructions, redeem_image, status, public_signup, completion_mode, required_food, required_game, created_at, festival_map_id)
  SELECT id, title, card_image, not_stamped_image, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(available_from, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(available_to, '')), details, redeem_instructions, redeem_image, status, public_signup, completion_mode, required_food, required_game, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), festival_map_id FROM stamp_rallies;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rallies) = (SELECT COUNT(*) FROM new_stamp_rallies), 'stamp_rallies: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rallies WHERE NULLIF(available_from, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rallies WHERE available_from IS NOT NULL), 'stamp_rallies.available_from: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rallies WHERE NULLIF(available_to, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rallies WHERE available_to IS NOT NULL), 'stamp_rallies.available_to: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rallies WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rallies WHERE created_at IS NOT NULL), 'stamp_rallies.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_stamp_rallies';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_stamp_rallies', seq FROM sqlite_sequence WHERE name = 'stamp_rallies';
DROP TABLE stamp_rallies;
ALTER TABLE new_stamp_rallies RENAME TO stamp_rallies;

-- stamp_rally_cards
CREATE TABLE new_stamp_rally_cards (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  rally_id INTEGER NOT NULL,
  token TEXT NOT NULL UNIQUE,
  participant_name TEXT NOT NULL DEFAULT '',
  completed INTEGER NOT NULL DEFAULT 0,
  completed_at DATETIME CHECK (completed_at IS strftime('%Y-%m-%dT%H:%M:%SZ', completed_at)),
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  world TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE
);
INSERT INTO new_stamp_rally_cards (id, rally_id, token, participant_name, completed, completed_at, created_at, world)
  SELECT id, rally_id, token, participant_name, completed, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(completed_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), world FROM stamp_rally_cards;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_cards) = (SELECT COUNT(*) FROM new_stamp_rally_cards), 'stamp_rally_cards: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_cards WHERE NULLIF(completed_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rally_cards WHERE completed_at IS NOT NULL), 'stamp_rally_cards.completed_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_cards WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rally_cards WHERE created_at IS NOT NULL), 'stamp_rally_cards.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_stamp_rally_cards';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_stamp_rally_cards', seq FROM sqlite_sequence WHERE name = 'stamp_rally_cards';
DROP TABLE stamp_rally_cards;
ALTER TABLE new_stamp_rally_cards RENAME TO stamp_rally_cards;
CREATE INDEX idx_stamp_rally_cards_participant ON stamp_rally_cards(participant_name COLLATE NOCASE);
CREATE INDEX idx_stamp_rally_cards_rally ON stamp_rally_cards(rally_id);
CREATE INDEX idx_stamp_rally_cards_token ON stamp_rally_cards(token);

-- stamp_rally_collected
CREATE TABLE new_stamp_rally_collected (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  rally_id INTEGER NOT NULL,
  card_id INTEGER,
  stamp_id INTEGER,
  participant_name TEXT NOT NULL DEFAULT '',
  stall_name TEXT NOT NULL DEFAULT '',
  stamp_type TEXT NOT NULL DEFAULT 'food',
  stamped_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (stamped_at IS strftime('%Y-%m-%dT%H:%M:%SZ', stamped_at)),
  UNIQUE (card_id, stamp_id),
  FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE,
  FOREIGN KEY (card_id) REFERENCES stamp_rally_cards(id) ON DELETE SET NULL,
  FOREIGN KEY (stamp_id) REFERENCES stamp_rally_stamps(id) ON DELETE SET NULL
);
INSERT INTO new_stamp_rally_collected (id, rally_id, card_id, stamp_id, participant_name, stall_name, stamp_type, stamped_at)
  SELECT id, rally_id, card_id, stamp_id, participant_name, stall_name, stamp_type, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(stamped_at, '')) FROM stamp_rally_collected;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_collected) = (SELECT COUNT(*) FROM new_stamp_rally_collected), 'stamp_rally_collected: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_collected WHERE NULLIF(stamped_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rally_collected WHERE stamped_at IS NOT NULL), 'stamp_rally_collected.stamped_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_stamp_rally_collected';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_stamp_rally_collected', seq FROM sqlite_sequence WHERE name = 'stamp_rally_collected';
DROP TABLE stamp_rally_collected;
ALTER TABLE new_stamp_rally_collected RENAME TO stamp_rally_collected;
CREATE INDEX idx_stamp_rally_collected_card ON stamp_rally_collected(card_id);
CREATE INDEX idx_stamp_rally_collected_rally ON stamp_rally_collected(rally_id);
CREATE INDEX idx_stamp_rally_collected_stamp ON stamp_rally_collected(stamp_id);

-- stamp_rally_stamps
CREATE TABLE new_stamp_rally_stamps (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  rally_id INTEGER NOT NULL,
  affiliate_id INTEGER,
  image TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  stamp_type TEXT NOT NULL DEFAULT 'food',
  pos_x REAL NOT NULL DEFAULT 0,
  pos_y REAL NOT NULL DEFAULT 0,
  width REAL NOT NULL DEFAULT 20,
  height REAL NOT NULL DEFAULT 20,
  rotation REAL NOT NULL DEFAULT 0,
  active_from DATETIME CHECK (active_from IS strftime('%Y-%m-%dT%H:%M:%SZ', active_from)),
  active_to DATETIME CHECK (active_to IS strftime('%Y-%m-%dT%H:%M:%SZ', active_to)),
  paused INTEGER NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  occupant_id INTEGER,
  FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE,
  FOREIGN KEY (affiliate_id) REFERENCES affiliates(id) ON DELETE SET NULL
);
INSERT INTO new_stamp_rally_stamps (id, rally_id, affiliate_id, image, password, stamp_type, pos_x, pos_y, width, height, rotation, active_from, active_to, paused, sort_order, occupant_id)
  SELECT id, rally_id, affiliate_id, image, password, stamp_type, pos_x, pos_y, width, height, rotation, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(active_from, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(active_to, '')), paused, sort_order, occupant_id FROM stamp_rally_stamps;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_stamps) = (SELECT COUNT(*) FROM new_stamp_rally_stamps), 'stamp_rally_stamps: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_stamps WHERE NULLIF(active_from, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rally_stamps WHERE active_from IS NOT NULL), 'stamp_rally_stamps.active_from: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM stamp_rally_stamps WHERE NULLIF(active_to, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_stamp_rally_stamps WHERE active_to IS NOT NULL), 'stamp_rally_stamps.active_to: times';
DELETE FROM sqlite_sequence WHERE name = 'new_stamp_rally_stamps';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_stamp_rally_stamps', seq FROM sqlite_sequence WHERE name = 'stamp_rally_stamps';
DROP TABLE stamp_rally_stamps;
ALTER TABLE new_stamp_rally_stamps RENAME TO stamp_rally_stamps;
CREATE INDEX idx_stamp_rally_stamps_occupant ON stamp_rally_stamps(occupant_id);
CREATE INDEX idx_stamp_rally_stamps_rally ON stamp_rally_stamps(rally_id);

-- styles
CREATE TABLE new_styles (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  board_flourish TEXT NOT NULL DEFAULT '',
  number_flourish TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  is_public INTEGER NOT NULL DEFAULT 0,
  tokens TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO new_styles (id, name, board_flourish, number_flourish, created_at, is_public, tokens)
  SELECT id, name, board_flourish, number_flourish, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), is_public, tokens FROM styles;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM styles) = (SELECT COUNT(*) FROM new_styles), 'styles: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM styles WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_styles WHERE created_at IS NOT NULL), 'styles.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_styles';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_styles', seq FROM sqlite_sequence WHERE name = 'styles';
DROP TABLE styles;
ALTER TABLE new_styles RENAME TO styles;

-- tea_rooms
CREATE TABLE new_tea_rooms (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  subtitle TEXT NOT NULL DEFAULT '',
  room_owner TEXT NOT NULL DEFAULT '',
  room_number TEXT NOT NULL DEFAULT '',
  cost_per_half_hour INTEGER NOT NULL DEFAULT 0,
  hashtags TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  seasonal INTEGER NOT NULL DEFAULT 0,
  open INTEGER NOT NULL DEFAULT 1,
  lockable INTEGER NOT NULL DEFAULT 0,
  discounted INTEGER NOT NULL DEFAULT 0,
  locked INTEGER NOT NULL DEFAULT 0,
  locked_until DATETIME CHECK (locked_until IS strftime('%Y-%m-%dT%H:%M:%SZ', locked_until)),
  image TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at))
);
INSERT INTO new_tea_rooms (id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description, seasonal, open, lockable, discounted, locked, locked_until, image, color, sort_order, created_at)
  SELECT id, name, subtitle, room_owner, room_number, cost_per_half_hour, hashtags, description, seasonal, open, lockable, discounted, locked, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(locked_until, '')), image, color, sort_order, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')) FROM tea_rooms;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM tea_rooms) = (SELECT COUNT(*) FROM new_tea_rooms), 'tea_rooms: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM tea_rooms WHERE NULLIF(locked_until, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_tea_rooms WHERE locked_until IS NOT NULL), 'tea_rooms.locked_until: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM tea_rooms WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_tea_rooms WHERE created_at IS NOT NULL), 'tea_rooms.created_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_tea_rooms';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_tea_rooms', seq FROM sqlite_sequence WHERE name = 'tea_rooms';
DROP TABLE tea_rooms;
ALTER TABLE new_tea_rooms RENAME TO tea_rooms;
CREATE UNIQUE INDEX idx_tea_rooms_room_number ON tea_rooms(room_number) WHERE room_number != '';
CREATE INDEX idx_tea_rooms_sort ON tea_rooms(sort_order, id);

-- user_passkeys
CREATE TABLE new_user_passkeys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id INTEGER NOT NULL,
  credential_id TEXT NOT NULL UNIQUE,
  credential TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  last_used_at DATETIME CHECK (last_used_at IS strftime('%Y-%m-%dT%H:%M:%SZ', last_used_at)),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
INSERT INTO new_user_passkeys (id, user_id, credential_id, credential, name, created_at, last_used_at)
  SELECT id, user_id, credential_id, credential, name, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(last_used_at, '')) FROM user_passkeys;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_passkeys) = (SELECT COUNT(*) FROM new_user_passkeys), 'user_passkeys: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_passkeys WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_user_passkeys WHERE created_at IS NOT NULL), 'user_passkeys.created_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_passkeys WHERE NULLIF(last_used_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_user_passkeys WHERE last_used_at IS NOT NULL), 'user_passkeys.last_used_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_user_passkeys';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_user_passkeys', seq FROM sqlite_sequence WHERE name = 'user_passkeys';
DROP TABLE user_passkeys;
ALTER TABLE new_user_passkeys RENAME TO user_passkeys;
CREATE INDEX idx_user_passkeys_user ON user_passkeys(user_id);

-- user_tokens
CREATE TABLE new_user_tokens (
  user_id INTEGER PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  token_prefix TEXT NOT NULL DEFAULT '',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  last_used_at DATETIME CHECK (last_used_at IS strftime('%Y-%m-%dT%H:%M:%SZ', last_used_at)),
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
INSERT INTO new_user_tokens (user_id, token_hash, token_prefix, created_at, last_used_at)
  SELECT user_id, token_hash, token_prefix, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(last_used_at, '')) FROM user_tokens;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_tokens) = (SELECT COUNT(*) FROM new_user_tokens), 'user_tokens: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_tokens WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_user_tokens WHERE created_at IS NOT NULL), 'user_tokens.created_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM user_tokens WHERE NULLIF(last_used_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_user_tokens WHERE last_used_at IS NOT NULL), 'user_tokens.last_used_at: times';
DROP TABLE user_tokens;
ALTER TABLE new_user_tokens RENAME TO user_tokens;

-- users
CREATE TABLE new_users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  is_admin INTEGER NOT NULL DEFAULT 0,
  is_active INTEGER NOT NULL DEFAULT 0,
  permissions TEXT NOT NULL DEFAULT '[]',
  created_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (created_at IS strftime('%Y-%m-%dT%H:%M:%SZ', created_at)),
  last_login_at DATETIME CHECK (last_login_at IS strftime('%Y-%m-%dT%H:%M:%SZ', last_login_at)),
  password_epoch INTEGER NOT NULL DEFAULT 0
);
INSERT INTO new_users (id, username, password_hash, is_admin, is_active, permissions, created_at, last_login_at, password_epoch)
  SELECT id, username, password_hash, is_admin, is_active, permissions, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(created_at, '')), strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(last_login_at, '')), password_epoch FROM users;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM users) = (SELECT COUNT(*) FROM new_users), 'users: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM users WHERE NULLIF(created_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_users WHERE created_at IS NOT NULL), 'users.created_at: times';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM users WHERE NULLIF(last_login_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_users WHERE last_login_at IS NOT NULL), 'users.last_login_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_users';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_users', seq FROM sqlite_sequence WHERE name = 'users';
DROP TABLE users;
ALTER TABLE new_users RENAME TO users;

-- winners_log
CREATE TABLE new_winners_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  logged_at DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')) CHECK (logged_at IS strftime('%Y-%m-%dT%H:%M:%SZ', logged_at)),
  card_id TEXT NOT NULL,
  player_name TEXT NOT NULL DEFAULT '',
  game_details TEXT NOT NULL DEFAULT '',
  winning_patterns TEXT NOT NULL DEFAULT '[]'
);
INSERT INTO new_winners_log (id, logged_at, card_id, player_name, game_details, winning_patterns)
  SELECT id, strftime('%Y-%m-%dT%H:%M:%SZ', NULLIF(logged_at, '')), card_id, player_name, game_details, winning_patterns FROM winners_log;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM winners_log) = (SELECT COUNT(*) FROM new_winners_log), 'winners_log: rows';
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM winners_log WHERE NULLIF(logged_at, '') IS NOT NULL) = (SELECT COUNT(*) FROM new_winners_log WHERE logged_at IS NOT NULL), 'winners_log.logged_at: times';
DELETE FROM sqlite_sequence WHERE name = 'new_winners_log';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_winners_log', seq FROM sqlite_sequence WHERE name = 'winners_log';
DROP TABLE winners_log;
ALTER TABLE new_winners_log RENAME TO winners_log;
CREATE INDEX idx_winners_log_logged_at ON winners_log(logged_at);
CREATE INDEX idx_winners_log_player_date ON winners_log(player_name, logged_at);

-- patterns
CREATE TABLE new_patterns (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  pattern_data TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0,
  category_id INTEGER NOT NULL DEFAULT 1,
  FOREIGN KEY (category_id) REFERENCES pattern_categories(id)
);
INSERT INTO new_patterns (id, name, pattern_data, sort_order, category_id)
  SELECT id, name, pattern_data, sort_order, category_id FROM patterns;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM patterns) = (SELECT COUNT(*) FROM new_patterns), 'patterns: rows';
DELETE FROM sqlite_sequence WHERE name = 'new_patterns';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_patterns', seq FROM sqlite_sequence WHERE name = 'patterns';
DROP TABLE patterns;
ALTER TABLE new_patterns RENAME TO patterns;

-- reading_list_items
CREATE TABLE new_reading_list_items (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  list_id INTEGER NOT NULL,
  cover_image TEXT NOT NULL DEFAULT '',
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  format TEXT NOT NULL DEFAULT '',
  genres TEXT NOT NULL DEFAULT '',
  tropes TEXT NOT NULL DEFAULT '',
  chapters TEXT NOT NULL DEFAULT '',
  comments TEXT NOT NULL DEFAULT '',
  sources TEXT NOT NULL DEFAULT '[]',
  sort_order INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (list_id) REFERENCES reading_lists(id) ON DELETE CASCADE
);
INSERT INTO new_reading_list_items (id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order)
  SELECT id, list_id, cover_image, title, summary, format, genres, tropes, chapters, comments, sources, sort_order FROM reading_list_items;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM reading_list_items) = (SELECT COUNT(*) FROM new_reading_list_items), 'reading_list_items: rows';
DELETE FROM sqlite_sequence WHERE name = 'new_reading_list_items';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_reading_list_items', seq FROM sqlite_sequence WHERE name = 'reading_list_items';
DROP TABLE reading_list_items;
ALTER TABLE new_reading_list_items RENAME TO reading_list_items;
CREATE INDEX idx_reading_list_items_list ON reading_list_items(list_id);

-- festival_stalls
CREATE TABLE new_festival_stalls (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  map_id INTEGER NOT NULL,
  shape TEXT NOT NULL DEFAULT 'rect',
  color TEXT NOT NULL DEFAULT '',
  selection_color TEXT NOT NULL DEFAULT '',
  text_color TEXT NOT NULL DEFAULT '',
  pos_x REAL NOT NULL DEFAULT 0,
  pos_y REAL NOT NULL DEFAULT 0,
  width REAL NOT NULL DEFAULT 12,
  height REAL NOT NULL DEFAULT 8,
  rotation REAL NOT NULL DEFAULT 0,
  sort_order INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (map_id) REFERENCES festival_maps(id) ON DELETE CASCADE
);
INSERT INTO new_festival_stalls (id, map_id, shape, color, selection_color, text_color, pos_x, pos_y, width, height, rotation, sort_order)
  SELECT id, map_id, shape, color, selection_color, text_color, pos_x, pos_y, width, height, rotation, sort_order FROM festival_stalls;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM festival_stalls) = (SELECT COUNT(*) FROM new_festival_stalls), 'festival_stalls: rows';
DELETE FROM sqlite_sequence WHERE name = 'new_festival_stalls';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_festival_stalls', seq FROM sqlite_sequence WHERE name = 'festival_stalls';
DROP TABLE festival_stalls;
ALTER TABLE new_festival_stalls RENAME TO festival_stalls;
CREATE INDEX idx_festival_stalls_map ON festival_stalls(map_id);

-- festival_stall_occupants
CREATE TABLE new_festival_stall_occupants (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  stall_id INTEGER NOT NULL,
  affiliate_id INTEGER,
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  event_carrd TEXT NOT NULL DEFAULT '',
  stall_type TEXT NOT NULL DEFAULT 'other',
  type_label TEXT NOT NULL DEFAULT '',
  times TEXT NOT NULL DEFAULT '[]',
  sort_order INTEGER NOT NULL DEFAULT 0,
  FOREIGN KEY (stall_id) REFERENCES festival_stalls(id) ON DELETE CASCADE,
  FOREIGN KEY (affiliate_id) REFERENCES affiliates(id) ON DELETE SET NULL
);
INSERT INTO new_festival_stall_occupants (id, stall_id, affiliate_id, title, description, event_carrd, stall_type, type_label, times, sort_order)
  SELECT id, stall_id, affiliate_id, title, description, event_carrd, stall_type, type_label, times, sort_order FROM festival_stall_occupants;
INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM festival_stall_occupants) = (SELECT COUNT(*) FROM new_festival_stall_occupants), 'festival_stall_occupants: rows';
DELETE FROM sqlite_sequence WHERE name = 'new_festival_stall_occupants';
INSERT INTO sqlite_sequence (name, seq) SELECT 'new_festival_stall_occupants', seq FROM sqlite_sequence WHERE name = 'festival_stall_occupants';
DROP TABLE festival_stall_occupants;
ALTER TABLE new_festival_stall_occupants RENAME TO festival_stall_occupants;
CREATE INDEX idx_festival_stall_occupants_stall ON festival_stall_occupants(stall_id);

INSERT INTO _migration_check SELECT (SELECT COUNT(*) FROM pragma_foreign_key_check) = 0, 'foreign keys';
DROP TABLE _migration_check;
PRAGMA user_version = 73;
COMMIT;
PRAGMA foreign_keys = ON;
