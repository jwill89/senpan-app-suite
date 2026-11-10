-- +goose Up
-- The sites whose pages may read the public tea-room API from a browser, a JSON array
-- of origins. A site's own domain and its subdomains are listed to begin with; saved
-- empty, the list allows no cross-origin read.
INSERT OR IGNORE INTO settings (key, value) VALUES ('tea_room_origins', '["https://senpan.cafe","https://*.senpan.cafe"]');
PRAGMA user_version = 74;
