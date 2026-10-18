-- Where an entry's words came from: 'user' (typed in Pocket Day) or
-- 'habits' (taken from the Pocket Habits journal on a day with nothing
-- written here). A habits entry follows Habits until it is edited here.
ALTER TABLE entries ADD COLUMN origin TEXT NOT NULL DEFAULT 'user';
