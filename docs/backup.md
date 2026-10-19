# Backup & restore

Everything lives in `DATA_DIR` (`/data` in the container):

- `pocket-day.db` (+ `-wal` / `-shm` while running) — entries, moments, pins, frozen facts, settings, users
- `backups/` — where the CLI and the UI write snapshots

The facts are a cache of other apps, but a valuable one: once those apps rotate or lose data, the frozen facts are the only record of what a day held. Back the database up like the journal it is.

## Consistent copies

Never copy a live WAL database file. Use one of:

- **UI**: Settings → Backup & data → *Download database* (admin only: it is the whole box)
- **API**: `GET /api/v1/backup` with an admin's bearer token
- **CLI**: `docker exec pocket-day pocket-day backup` → prints the path under `/data/backups/`
- **sqlite3**: `sqlite3 data/pocket-day.db "VACUUM INTO 'snap.db'"`

## Restore

1. Stop the container.
2. Replace `data/pocket-day.db` with the snapshot (remove any stale `-wal` / `-shm`).
3. Start the container. Migrations run forward automatically.

## Export

- `GET /api/v1/export.json` — your entries, moments, pins and your facts. Import merges into your own journal: entries by date without overwriting, moments and pins when missing, facts only for days that have none.
- `GET /api/v1/export.md` — the words alone, one heading per day.
