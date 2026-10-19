-- Everything a person sees is theirs alone. Entries, moments, pins, stories
-- and questions already carried user_id; this makes the rest personal too:
-- the facts (they were gathered with one person's source tokens), the
-- settings that are a person's (sources, AI key, notifications, home place,
-- journal options), the AI token counter and the "already pushed today"
-- markers. Everything that exists today belongs to the owner: the first admin
-- (members never had a way to own data). On a database with no users yet the
-- old rows stay where they are and the first account adopts them.

-- facts: (date, source) → (user_id, date, source)
CREATE TABLE facts_new (
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  date       TEXT NOT NULL,
  source     TEXT NOT NULL,
  payload    TEXT NOT NULL DEFAULT '',
  headline   REAL NOT NULL DEFAULT 0,
  summary    TEXT NOT NULL DEFAULT '',
  error      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL,
  PRIMARY KEY (user_id, date, source)
);
INSERT INTO facts_new (user_id, date, source, payload, headline, summary, error, fetched_at)
  SELECT o.id, f.date, f.source, f.payload, f.headline, f.summary, f.error, f.fetched_at
  FROM facts f, (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1) o;
DROP TABLE facts;
ALTER TABLE facts_new RENAME TO facts;
CREATE INDEX facts_user_source_date ON facts(user_id, source, date);

-- personal settings; the settings table keeps only the box-wide ones (title, setup_complete)
CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);
INSERT OR IGNORE INTO user_settings (user_id, key, value)
  SELECT o.id, s.key, s.value
  FROM settings s, (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1) o
  WHERE s.key NOT IN ('title', 'setup_complete');
DELETE FROM settings
  WHERE key NOT IN ('title', 'setup_complete')
    AND EXISTS (SELECT 1 FROM users);

-- AI usage rows without an owner were the owner's calls
UPDATE ai_usage SET user_id = (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1)
  WHERE user_id IS NULL OR user_id = 0;
CREATE INDEX ai_usage_user_created ON ai_usage(user_id, created_at);

-- "last day pushed" markers: notify:<kind> → notify:<user>:<kind>
UPDATE cache SET key = 'notify:' || (SELECT id FROM users ORDER BY CASE WHEN role = 'admin' THEN 0 ELSE 1 END, id LIMIT 1) || ':' || substr(key, 8)
  WHERE key IN ('notify:evening', 'notify:morning', 'notify:weekly')
    AND EXISTS (SELECT 1 FROM users);
