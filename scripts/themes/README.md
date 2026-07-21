# Theme seed pack — WCAG AAA styles

A set of **28 themes** (a **light** and **dark** variant of each of 14 concepts) for
the App Suite theme system, plus a PHP script that inserts them into the `styles`
table of the SQLite database.

## Concepts

| # | Concept | Light | Dark |
|---|---------|-------|------|
| 1 | Summer Beachy Blues | ✓ | ✓ |
| 2 | Spring Sakura Pinks & Greens | ✓ | ✓ |
| 3 | Autumn Leaves | ✓ | ✓ |
| 4 | Winter Colors | ✓ | ✓ |
| 5 | Mysticism & Magic | ✓ | ✓ |
| 6 | La Noscea (FF14) | ✓ | ✓ |
| 7 | Thanalan (FF14) | ✓ | ✓ |
| 8 | The Shroud (FF14) | ✓ | ✓ |
| 9 | Ishgard (FF14) | ✓ | ✓ |
| 10 | Sharlayan (FF14) | ✓ | ✓ |
| 11 | Doma & Hingashi (FF14) | ✓ | ✓ |
| 12 | Radz-at-Han (FF14) | ✓ | ✓ |
| 13 | Tuliyollal (FF14) | ✓ | ✓ |
| 14 | Solution Nine (FF14) | ✓ | ✓ |

## Files

- **`insert_styles.php`** — self-contained inserter (the 28 themes are embedded, so
  it's the only file you need on the host). Inserts each theme as a row in `styles`
  with its design tokens JSON-encoded into the `tokens` column — the same shape the
  Go server reads and generates CSS from.
- **`themes.json`** — the readable source of truth for the 28 token maps.

## Usage

```bash
# Back up the DB first — this writes to the live styles table.
cp /opt/senpan/data/database.sqlite /opt/senpan/data/database.sqlite.bak

php insert_styles.php /opt/senpan/data/database.sqlite
```

- **Idempotent:** a theme whose `name` already exists is skipped, so re-running is
  safe and won't create duplicates.
- **Transactional:** all inserts run in one transaction (all-or-nothing).
- Requires PHP with the PDO SQLite driver (`pdo_sqlite`).
- It only ever inserts new rows — it never touches existing themes, the active-style
  setting, or any other table. Nothing is activated; pick a theme in the admin UI as
  usual.

Tip: the app holds a connection to the DB while running. Either run this during a
quiet window, or stop the service first (`systemctl stop senpan`), run the insert,
then start it again — the same reason the deploy DB-refresh stops the service.

## WCAG AAA

Every theme meets **WCAG 2.1 AAA** contrast (7:1 for normal text, 4.5:1 for large
text) across all of the app's foreground/background token pairs. This was verified
end-to-end with the backend's own checker against a copy of the live DB after the
PHP insert:

```bash
go run ./backend/cmd/themetool check <db>   # -> AA fails: 0   AAA fails: 0 (for all 28)
```

If you edit `themes.json`, re-check with `themetool` before shipping.
