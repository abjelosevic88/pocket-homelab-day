CREATE TABLE users (
  id            INTEGER PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'admin',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

-- Small key/value cache for things like "last notification day".
CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);

-- The day's own words: one entry per user per date. Everything the user
-- typed lives here; everything the other apps know lives in facts.
CREATE TABLE entries (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,                         -- YYYY-MM-DD
  highlight  TEXT NOT NULL DEFAULT '',              -- the one line
  text       TEXT NOT NULL DEFAULT '',              -- the longer note (markdown-ish plain text)
  mood       INTEGER NOT NULL DEFAULT 0,            -- 0 unset, 1 (rough) .. 5 (great)
  tags       TEXT NOT NULL DEFAULT '',              -- comma separated, lower case
  pinned     INTEGER NOT NULL DEFAULT 0,            -- a day worth coming back to
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, date)
);
CREATE INDEX entries_date ON entries(date);

-- Short timestamped lines through the day ("coffee with M.", "finished the
-- deck"). A phone shortcut posts them; the day page lists them.
CREATE TABLE moments (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  time       TEXT NOT NULL DEFAULT '',              -- HH:MM local, '' when unknown
  text       TEXT NOT NULL,
  kind       TEXT NOT NULL DEFAULT 'note',          -- note | win | thanks | idea | event
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX moments_user_date ON moments(user_id, date, time);

-- Things pulled from another app that the user chose to keep on the day:
-- a photo, a song, a place, a transaction. ref identifies it in its source.
CREATE TABLE pins (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  source     TEXT NOT NULL,
  ref        TEXT NOT NULL DEFAULT '',
  title      TEXT NOT NULL DEFAULT '',
  subtitle   TEXT NOT NULL DEFAULT '',
  image      TEXT NOT NULL DEFAULT '',              -- a URL the UI can show (proxied for Immich)
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  UNIQUE (user_id, date, source, ref)
);
CREATE INDEX pins_user_date ON pins(user_id, date);

-- What the other apps knew about a day, one row per source. Past days are
-- frozen once fetched, so the journal still reads when a source is gone.
CREATE TABLE facts (
  date       TEXT NOT NULL,
  source     TEXT NOT NULL,
  payload    TEXT NOT NULL DEFAULT '',              -- JSON, source specific
  headline   REAL NOT NULL DEFAULT 0,               -- the one number: photos, km, listens, minutes, spend…
  summary    TEXT NOT NULL DEFAULT '',              -- one line for lists and pushes
  error      TEXT NOT NULL DEFAULT '',              -- last fetch error, '' when fine
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (date, source)
);
CREATE INDEX facts_source_date ON facts(source, date);
