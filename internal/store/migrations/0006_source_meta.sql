-- A source an app brought itself (it offers "what happened on a day" through
-- Pocket Core, and Day found it there) has no entry in Day's own tables: its
-- label, the app's name and icon and how its things are counted come from the
-- app. They are kept here per person, written whenever a fact of the source
-- is, so a day recorded long ago still reads well when the app has gone or
-- Pocket Core is away.
CREATE TABLE source_meta (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  source     TEXT NOT NULL,
  label      TEXT NOT NULL DEFAULT '',
  app        TEXT NOT NULL DEFAULT '',
  icon       TEXT NOT NULL DEFAULT '',
  one        TEXT NOT NULL DEFAULT '',
  many       TEXT NOT NULL DEFAULT '',
  unit       TEXT NOT NULL DEFAULT '',
  what       TEXT NOT NULL DEFAULT '',
  url        TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  PRIMARY KEY (user_id, source)
);
