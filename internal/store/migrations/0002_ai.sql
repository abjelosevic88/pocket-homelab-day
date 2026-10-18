-- Stories the AI wrote for a period: a week (2026-W42), a month (2026-10) or a year (2026).
CREATE TABLE ai_stories (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       TEXT NOT NULL,            -- week | month | year
  period     TEXT NOT NULL,
  title      TEXT NOT NULL DEFAULT '',
  text       TEXT NOT NULL DEFAULT '',
  model      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  PRIMARY KEY (user_id, kind, period)
);

-- "Ask your journal": every question with its answer and the days it used.
CREATE TABLE ai_asks (
  id         INTEGER PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  question   TEXT NOT NULL,
  answer     TEXT NOT NULL DEFAULT '',
  dates      TEXT NOT NULL DEFAULT '[]', -- JSON [YYYY-MM-DD] the answer cites
  scope      TEXT NOT NULL DEFAULT '',   -- "2026-09-01..2026-10-18 · lake"
  model      TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX ai_asks_user ON ai_asks(user_id, id);

-- Token usage per call, for the Settings → AI counter.
CREATE TABLE ai_usage (
  id                INTEGER PRIMARY KEY,
  user_id           INTEGER,
  feature           TEXT NOT NULL,
  model             TEXT NOT NULL DEFAULT '',
  prompt_tokens     INTEGER NOT NULL DEFAULT 0,
  completion_tokens INTEGER NOT NULL DEFAULT 0,
  created_at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX ai_usage_created ON ai_usage(created_at);
