-- Several users. Everything that belongs to a person gets a user_id; a former
-- single-user installation becomes user 1, an admin with the same login.
CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    display_name  TEXT NOT NULL DEFAULT '',
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    created_at    TEXT NOT NULL,
    last_login_at TEXT,
    disabled      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE invites (
    token      TEXT PRIMARY KEY, -- sha256 of the link token
    created_by INTEGER REFERENCES users (id) ON DELETE SET NULL,
    role       TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('admin', 'user')),
    expires_at TEXT NOT NULL,
    used_by    INTEGER REFERENCES users (id) ON DELETE SET NULL,
    used_at    TEXT
);

INSERT INTO users (id, username, password_hash, role, created_at)
SELECT 1,
       coalesce((SELECT value FROM settings WHERE key = 'admin_user'), 'admin'),
       coalesce((SELECT value FROM settings WHERE key = 'password_hash'), ''),
       'admin',
       strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE EXISTS (SELECT 1 FROM settings WHERE key = 'password_hash') OR EXISTS (SELECT 1 FROM habits);

-- Without a stored name the start can still apply HABITD_ADMIN_USER once.
INSERT INTO settings (key, value)
SELECT 'legacy_admin_unnamed', '1'
WHERE EXISTS (SELECT 1 FROM users) AND NOT EXISTS (SELECT 1 FROM settings WHERE key = 'admin_user');

CREATE TABLE user_settings (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key     TEXT NOT NULL,
    value   TEXT NOT NULL,
    PRIMARY KEY (user_id, key)
) WITHOUT ROWID;

INSERT INTO user_settings (user_id, key, value)
SELECT 1, key, value FROM settings
WHERE key NOT IN ('password_hash', 'admin_user', 'legacy_admin_unnamed', 'vapid_public', 'vapid_private')
  AND EXISTS (SELECT 1 FROM users WHERE id = 1);

DELETE FROM settings WHERE key NOT IN ('vapid_public', 'vapid_private', 'legacy_admin_unnamed');

ALTER TABLE areas ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE habits ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE checkins ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE habit_entries ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE relapses ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE rewards ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE redemptions ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE ledger ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE celebrations ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE sessions ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;
ALTER TABLE push_subscriptions ADD COLUMN user_id INTEGER REFERENCES users (id) ON DELETE CASCADE;

UPDATE areas SET user_id = 1 WHERE EXISTS (SELECT 1 FROM users WHERE id = 1);
UPDATE habits SET user_id = 1;
UPDATE checkins SET user_id = 1;
UPDATE habit_entries SET user_id = 1;
UPDATE relapses SET user_id = 1;
UPDATE rewards SET user_id = 1;
UPDATE redemptions SET user_id = 1;
UPDATE ledger SET user_id = 1;
UPDATE celebrations SET user_id = 1;
UPDATE push_subscriptions SET user_id = 1;
UPDATE sessions SET user_id = 1 WHERE EXISTS (SELECT 1 FROM users WHERE id = 1);
DELETE FROM sessions WHERE user_id IS NULL;

-- badges_unlocked keeps one row per user and badge.
CREATE TABLE badges_unlocked_new (
    user_id     INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    badge_key   TEXT NOT NULL,
    unlocked_at TEXT NOT NULL,
    PRIMARY KEY (user_id, badge_key)
) WITHOUT ROWID;
INSERT INTO badges_unlocked_new (user_id, badge_key, unlocked_at)
SELECT 1, badge_key, unlocked_at FROM badges_unlocked WHERE EXISTS (SELECT 1 FROM users WHERE id = 1);
DROP TABLE badges_unlocked;
ALTER TABLE badges_unlocked_new RENAME TO badges_unlocked;

-- Rows without a user are the global defaults, new users get a copy.
DELETE FROM areas WHERE user_id IS NULL;
INSERT INTO areas (key, name, color, icon, sort) VALUES
    ('gesundheit',  'Gesundheit',  '#5FD3C6', 'heart',     1),
    ('lernen',      'Lernen',      '#F28BB0', 'book',      2),
    ('achtsamkeit', 'Achtsamkeit', '#7FB8F0', 'lotus',     3),
    ('gute-taten',  'Gute Taten',  '#E8A0E8', 'handheart', 4),
    ('familie',     'Familie',     '#F2B880', 'home',      5),
    ('freunde',     'Freunde',     '#F5A3A3', 'users',     6),
    ('arbeit',      'Arbeit',      '#9FB4C8', 'brief',     7),
    ('digital',     'Digital',     '#8C8CF5', 'phoneoff',  8),
    ('geld',        'Geld',        '#D9B26B', 'wallet',    9),
    ('umwelt',      'Umwelt',      '#8ACB7A', 'leaf',      10);

CREATE INDEX areas_user ON areas (user_id);
CREATE INDEX habits_user ON habits (user_id);
CREATE INDEX checkins_user ON checkins (user_id);
CREATE INDEX habit_entries_user ON habit_entries (user_id);
CREATE INDEX relapses_user ON relapses (user_id);
CREATE INDEX rewards_user ON rewards (user_id);
CREATE INDEX redemptions_user ON redemptions (user_id);
CREATE INDEX ledger_user ON ledger (user_id);
CREATE INDEX celebrations_user ON celebrations (user_id);
CREATE INDEX sessions_user ON sessions (user_id);
CREATE INDEX push_subscriptions_user ON push_subscriptions (user_id);
