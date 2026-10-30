CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE areas (
    id    INTEGER PRIMARY KEY,
    key   TEXT NOT NULL DEFAULT '',
    name  TEXT NOT NULL,
    color TEXT NOT NULL,
    icon  TEXT NOT NULL DEFAULT '',
    sort  INTEGER NOT NULL DEFAULT 0
);

INSERT INTO areas (key, name, color, icon, sort) VALUES
    ('gesundheit',  'Gesundheit',  '#5FD3C6', 'heart',      1),
    ('lernen',      'Lernen',      '#F28BB0', 'book',       2),
    ('achtsamkeit', 'Achtsamkeit', '#7FB8F0', 'lotus',      3),
    ('gute-taten',  'Gute Taten',  '#E8A0E8', 'hand-heart', 4),
    ('familie',     'Familie',     '#F2B880', 'house',      5),
    ('freunde',     'Freunde',     '#7FB8F0', 'users',      6),
    ('arbeit',      'Arbeit',      '#9FB4C8', 'briefcase',  7);

CREATE TABLE habits (
    id            INTEGER PRIMARY KEY,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('build', 'avoid')),
    frequency     TEXT NOT NULL CHECK (frequency IN ('daily', 'weekly', 'custom')),
    weekly_target INTEGER NOT NULL DEFAULT 0,
    days_mask     INTEGER NOT NULL DEFAULT 0,
    icon          TEXT NOT NULL DEFAULT '',
    tag           TEXT NOT NULL DEFAULT '',
    area_id       INTEGER NOT NULL REFERENCES areas (id),
    difficulty    TEXT NOT NULL DEFAULT 'easy' CHECK (difficulty IN ('easy', 'medium', 'hard')),
    archived      INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    sort          INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE checkins (
    habit_id   INTEGER NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    date       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    used_joker INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (habit_id, date)
) WITHOUT ROWID;

CREATE TABLE relapses (
    habit_id INTEGER NOT NULL REFERENCES habits (id) ON DELETE CASCADE,
    date     TEXT NOT NULL,
    PRIMARY KEY (habit_id, date)
) WITHOUT ROWID;

CREATE TABLE rewards (
    id               INTEGER PRIMARY KEY,
    title            TEXT NOT NULL,
    mode             TEXT NOT NULL CHECK (mode IN ('coins', 'streak')),
    price            INTEGER NOT NULL DEFAULT 0,
    habit_id         INTEGER REFERENCES habits (id) ON DELETE SET NULL,
    streak_target    INTEGER NOT NULL DEFAULT 0,
    reset_on_relapse INTEGER NOT NULL DEFAULT 0,
    repeatable       INTEGER NOT NULL DEFAULT 0,
    archived         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE redemptions (
    id          INTEGER PRIMARY KEY,
    reward_id   INTEGER NOT NULL REFERENCES rewards (id) ON DELETE CASCADE,
    redeemed_at TEXT NOT NULL,
    price_paid  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE ledger (
    id     INTEGER PRIMARY KEY,
    at     TEXT NOT NULL,
    kind   TEXT NOT NULL CHECK (kind IN ('xp', 'coins', 'joker')),
    amount INTEGER NOT NULL,
    reason TEXT NOT NULL,
    ref    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX ledger_ref ON ledger (ref);
CREATE INDEX ledger_at ON ledger (at);

CREATE TABLE badges_unlocked (
    badge_key   TEXT PRIMARY KEY,
    unlocked_at TEXT NOT NULL
);

-- Celebrations waiting to be shown: level ups and new badges.
CREATE TABLE celebrations (
    id      INTEGER PRIMARY KEY,
    kind    TEXT NOT NULL CHECK (kind IN ('level', 'badge')),
    payload TEXT NOT NULL
);

CREATE TABLE sessions (
    token_hash TEXT PRIMARY KEY,
    csrf       TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE TABLE push_subscriptions (
    endpoint TEXT PRIMARY KEY,
    p256dh   TEXT NOT NULL,
    auth     TEXT NOT NULL
);
